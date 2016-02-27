#include <cassert>
#include <cstdlib>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include <llvm/IR/Function.h>
#include <llvm/IR/IRBuilder.h>
#include <llvm/IR/InlineAsm.h>
#include <llvm/IR/Instructions.h>
#include <llvm/IR/Intrinsics.h>
#include <llvm/IR/Metadata.h>
#include <llvm/IR/Module.h>
#include <llvm/IR/Operator.h>
#include <llvm/Pass.h>
#include <llvm/Support/raw_ostream.h>

#define GATE_NUM_ARGS_RESERVE 16 // must be >= GATE_NUM_ARGS in gate/args.h

using namespace llvm;

namespace {

typedef std::vector<Instruction *> InstrVector;
typedef std::unordered_map<Value *, InstrVector> MemInstrMap;
typedef std::unordered_set<Function *> FunctionSet;

static bool is_valid_function(Function *f, FunctionSet &replace_intrinsics)
{
	switch (f->getIntrinsicID()) {
	case Intrinsic::annotation:
	case Intrinsic::bswap:
	case Intrinsic::dbg_declare:
	case Intrinsic::dbg_value:
	case Intrinsic::fabs:
	case Intrinsic::lifetime_end:
	case Intrinsic::lifetime_start:
	case Intrinsic::not_intrinsic:
		return true;

	case Intrinsic::memcpy:
	case Intrinsic::memset:
		replace_intrinsics.insert(f);
		return true;

	default:
		return false;
	}
}

static void scan_instrs(Function &scanned_func, FunctionSet &replace_intrinsics, MemInstrMap &mem_accesses, MemInstrMap &mem_others, FunctionSet &indirect_funcs, InstrVector &indirect_calls)
{
	for (BasicBlock &bb: scanned_func) {
		for (Instruction &instr: bb) {
			bool access = instr.mayReadOrWriteMemory();
			auto call = dyn_cast<CallInst>(&instr);
			bool direct_call = false;

			if (call) {
				if (call->getCalledFunction()) {
					direct_call = true;
				} else if (auto expr = dyn_cast<ConstantExpr>(call->getCalledValue())) {
					direct_call = expr->isCast() && isa<Function>(expr->getOperand(0));
				}
			}

			for (Value *op: instr.operand_values()) {
				if (isa<InlineAsm>(op)) {
					errs() << "Inline assembly not allowed\n";
					std::exit(90);
					throw;
				}

				if (auto op_func = dyn_cast<Function>(op)) {
					if (!is_valid_function(op_func, replace_intrinsics)) {
						errs() << "Intrinsic function not allowed: ";
						errs().write_escaped(op_func->getName()) << "\n";
						std::exit(91);
						throw;
					}
				}

				if (op->getType()->isPointerTy() && !isa<GlobalValue>(op)) {
					if (access && !call)
						mem_accesses[op].push_back(&instr);
					else
						mem_others[op].push_back(&instr);
				}

				if (!direct_call) {
					// all operands of indirect calls
					if (auto func_ptr = dyn_cast<Function>(op))
						indirect_funcs.insert(func_ptr);
				}
			}

			if (direct_call) {
				// only arguments of direct calls
				for (Value *op: call->arg_operands())
					if (auto func_ptr = dyn_cast<Function>(op))
						indirect_funcs.insert(func_ptr);
			}

			if (call && !direct_call)
				indirect_calls.push_back(&instr);
		}
	}
}

static void modify_mem_accesses(Function &f, const MemInstrMap &mem_accesses, const MemInstrMap &mem_others)
{
	auto i32_type = Type::getInt32Ty(f.getContext());

	for (auto &pair: mem_accesses) {
		Value *op = pair.first;
		const InstrVector &accesses = pair.second;

		BasicBlock *bb;
		BasicBlock::iterator pos;

		if (auto op_instr = dyn_cast<Instruction>(op)) {
			bb = op_instr->getParent();
			pos = bb->begin();

			while (&(*pos) != op_instr) // find the original
				++pos;                  //

			++pos;                      // insert after the original

			while (isa<PHINode>(*pos))  // skip all phi instructions, if any
				++pos;                  //
		} else if (isa<Argument>(op) || isa<Constant>(op)) { // TODO: modify constants at build time
			bb = &f.getEntryBlock();
			pos = bb->getFirstInsertionPt();
		} else {
			errs() << "Unsupported memory access operand: " << *op << "\n";
			std::exit(93);
			throw;
		}

		auto name = op->getName();
		if (name.empty())
			name = "ptr";

		IRBuilder<> b(bb, pos);

		auto ptrToI32 = b.CreatePtrToInt(op, i32_type, name+".i32");
		auto i32ToPtr = b.CreateIntToPtr(ptrToI32, op->getType(), name+".safe");

		for (Instruction *user: accesses)
			user->replaceUsesOfWith(op, i32ToPtr);

		// replace calls' arguments, except if it would be the only thing to do
		auto i = mem_others.find(op);
		if (i != mem_others.end()) {
			const InstrVector &calls = i->second;
			
			for (Instruction *user: calls)
				user->replaceUsesOfWith(op, i32ToPtr);
		}
	}
}

static void modify_indirect_calls(Function &f, Instruction *check_proto, const InstrVector &calls)
{
	if (calls.empty())
		return;

	BasicBlock *first_bb = nullptr;
	bool multi_bb = false;
	Instruction *entry_bb_pos = nullptr;

	for (Instruction *instr: calls) {
		BasicBlock *bb = instr->getParent();

		if (!first_bb)
			first_bb = bb;
		else if (first_bb != bb)
			multi_bb = true;

		if (!entry_bb_pos && bb == &f.getEntryBlock())
			entry_bb_pos = instr;
	}

	auto check = check_proto->clone();
	bool check_inserted = false;

	if (multi_bb) {
		if (entry_bb_pos) {
			IRBuilder<> b(entry_bb_pos);

			b.Insert(check, check_proto->getName());
		} else {
			BasicBlock &bb = f.getEntryBlock();
			IRBuilder<> b(&bb, bb.getFirstInsertionPt());

			b.Insert(check, check_proto->getName());
		}

		check_inserted = true;
	}

	for (Instruction *instr: calls) {
		auto call = cast<CallInst>(instr);
		auto target = call->getCalledValue();
		auto target_type = target->getType();
		auto voidptr_type = PointerType::getUnqual(Type::getInt8Ty(f.getContext()));

		auto name = target->getName();
		if (name.empty())
			name = "funcptr";

		IRBuilder<> b(instr);

		if (!check_inserted) {
			b.Insert(check, check_proto->getName());
			check_inserted = true;
		}

		auto voidptr_target = b.CreatePointerCast(target, voidptr_type, name+".void");
		auto voidptr_safe_target = b.CreateCall(check, {voidptr_target}, name+".void.safe");
		auto safe_target = b.CreatePointerCast(voidptr_safe_target, target_type, name+".safe");

		call->replaceUsesOfWith(target, safe_target);
	}
}

static void handle_function(Function &f, FunctionSet &replace_intrinsics, FunctionSet &indirect_funcs, Instruction *indirect_check_proto)
{
	if (f.hasPrologueData()) {
		errs() << "Function prologue not allowed\n";
		std::exit(89);
		throw;
	}

	f.addFnAttr(Attribute::SafeStack);

	if (!f.isDeclaration()) {
		MemInstrMap mem_accesses;
		MemInstrMap mem_others;
		InstrVector indirect_calls;

		scan_instrs(f, replace_intrinsics, mem_accesses, mem_others, indirect_funcs, indirect_calls);

		modify_mem_accesses(f, mem_accesses, mem_others);
		modify_indirect_calls(f, indirect_check_proto, indirect_calls);
	}
}

static Function *lookup_impl(Module &m, FunctionType *type, StringRef name)
{
	if (auto gv = m.getNamedValue(name))
		if (auto f = dyn_cast<Function>(gv))
			if (!f->isDeclaration() && !f->isIntrinsic() && f->getFunctionType() == type)
				return f;

	return nullptr;
}

static void modify_intrinsic_calls(Module &m, const FunctionSet &replace_intrinsics)
{
	auto i32_type = Type::getInt32Ty(m.getContext());
	auto i64_type = Type::getInt64Ty(m.getContext());

	for (Function *intrin: replace_intrinsics) {
		auto id = intrin->getIntrinsicID();
		auto type = intrin->getFunctionType();
		Function *impl = nullptr;

		switch (id) {
		case Intrinsic::memcpy:
			if (type->getNumParams() == 5) {
				auto size_type = type->getParamType(2);

				if (size_type == i32_type)
					impl = lookup_impl(m, type, "__gate_memcpy_p0i8_p0i8_i32");
				else if (size_type == i64_type)
					impl = lookup_impl(m, type, "__gate_memcpy_p0i8_p0i8_i64");
			}
			break;

		case Intrinsic::memset:
			if (type->getNumParams() == 5) {
				auto size_type = type->getParamType(2);

				if (size_type == i32_type)
					impl = lookup_impl(m, type, "__gate_memset_p0i8_i32");
				else if (size_type == i64_type)
					impl = lookup_impl(m, type, "__gate_memset_p0i8_i64");
			}
			break;

		default:
			assert(false);
			throw;
		}

		if (!impl) {
			errs() << "Intrinsic function not implemented: ";
			errs().write_escaped(intrin->getName()) << "\n";
			std::exit(91);
			throw;
		}

		intrin->replaceAllUsesWith(impl);
	}
}

static GlobalValue *check_name_declaration(Module &m, const StringRef &name)
{
	if (auto v = m.getNamedValue(name)) {
		if (v->isDeclarationForLinker())
			return v;

		errs() << "Variable name is reserved: " << name << "\n";
		std::exit(87);
		throw;
	}

	return nullptr;
}

static void check_reserved_name(Module &m, const StringRef &name)
{
	if (check_name_declaration(m, name)) {
		errs() << "Variable access not allowed: " << name << "\n";
		std::exit(87);
		throw;
	}
}

static Instruction *create_indirect_call_check_instr(Module &m)
{
	StringRef name = "__gate_indirect_call_check";
	check_reserved_name(m, name);

	LLVMContext &ctx = m.getContext();

	auto i8_type = Type::getInt8Ty(ctx);
	auto i64_type = Type::getInt64Ty(ctx);
	auto voidptr_type = PointerType::getUnqual(i8_type);
	auto func_type = FunctionType::get(voidptr_type, {voidptr_type});
	auto funcptr_type = PointerType::getUnqual(func_type);

	auto null = Constant::getNullValue(funcptr_type);

	auto var = new GlobalVariable(m, funcptr_type, false, GlobalValue::ExternalLinkage, null, name);
	auto instr = new LoadInst(var, "indirect_call_check");

	auto empty_md = MDNode::get(ctx, {});
	auto deref_md = MDNode::get(ctx, { ConstantAsMetadata::get(ConstantInt::get(i64_type, 8)) });
	auto align_md = MDNode::get(ctx, { ConstantAsMetadata::get(ConstantInt::get(i64_type, 16)) });

	instr->setMetadata("invariant.load", empty_md);
	instr->setMetadata("nonnull", empty_md);
	instr->setMetadata("dereferenceable", deref_md);
	instr->setMetadata("align", align_md);

	return instr;
}

static void create_indirect_func_array(Module &m, const FunctionSet &indirect_funcs)
{
	StringRef name = "__gate_indirect_functions";
	check_reserved_name(m, name);

	auto ptr_type = PointerType::getUnqual(Type::getInt8Ty(m.getContext()));

	std::vector<Constant *> ptrs;
	ptrs.reserve(indirect_funcs.size());

	for (Constant *f: indirect_funcs)
		ptrs.push_back(ConstantExpr::getPointerCast(f, ptr_type));

	auto array_type = ArrayType::get(ptr_type, ptrs.size());
	auto array = ConstantArray::get(array_type, ptrs);

	auto var = new GlobalVariable(m, array_type, true, GlobalValue::ExternalLinkage, array, name);
	var->setSection(".gate");
}

static void create_args_ptr(Module &m)
{
	StringRef name = "__gate_args";
	StringRef tmp_name = "__gate_args_decl";

	auto decl = check_name_declaration(m, name);
	if (decl)
		decl->setName(tmp_name);

	auto type = ArrayType::get(Type::getInt32Ty(m.getContext()), GATE_NUM_ARGS_RESERVE);
	auto null = Constant::getNullValue(type);

	auto var = new GlobalVariable(m, type, false, GlobalValue::ExternalLinkage, null, name);

	if (decl) {
		decl->replaceAllUsesWith(ConstantExpr::getPointerCast(var, decl->getType()));
		var->setName(name);
	}
}

static void create_safestack_support(Module &m)
{
	StringRef name = "__safestack_unsafe_stack_ptr";
	check_reserved_name(m, name);

	auto type = PointerType::getUnqual(Type::getInt8Ty(m.getContext()));
	auto null = Constant::getNullValue(type);

	new GlobalVariable(m, type, false, GlobalValue::ExternalLinkage, null, name, nullptr, GlobalValue::GeneralDynamicTLSModel);
}

static void modify_global_value(GlobalValue &g)
{
	if (g.hasSection()) {
		errs() << "Custom section not allowed\n";
		std::exit(88);
		throw;
	}

	if (g.hasDLLImportStorageClass()) {
		errs() << "DLL import not allowed\n";
		std::exit(88);
		throw;
	}

	if (g.hasComdat())
		g.setVisibility(GlobalValue::HiddenVisibility);
	else
		g.setVisibility(GlobalValue::DefaultVisibility);

	g.setDLLStorageClass(GlobalValue::DefaultStorageClass);
	g.setThreadLocal(false);
}

static void modify_globals(Module &m)
{
	if (!m.getModuleInlineAsm().empty()) {
		errs() << "Inline assembly not allowed\n";
		std::exit(88);
		throw;
	}

	for (GlobalVariable &var: m.globals()) {
		if (var.getType()->getAddressSpace() != 0) {
			errs() << "Multiple address spaces not allowed\n";
			std::exit(88);
			throw;
		}

		modify_global_value(var);

		if (!var.isDeclarationForLinker() && !var.hasComdat())
			var.setLinkage(GlobalValue::PrivateLinkage);
	}

	for (Function &f: m) {
		modify_global_value(f);

		if (!f.isDeclarationForLinker() && !f.hasComdat() && !f.isIntrinsic() && f.getName() != "_start")
			f.setLinkage(GlobalValue::PrivateLinkage);
	}

	for (GlobalAlias &a: m.aliases()) {
		modify_global_value(a);
		a.setLinkage(GlobalValue::PrivateLinkage);
	}
}

static void handle_module(Module &m)
{
	modify_globals(m);

	FunctionSet replace_intrinsics;
	FunctionSet indirect_funcs;

	Instruction *indirect_check_proto = create_indirect_call_check_instr(m);

	for (Function &f: m)
		if (!f.isIntrinsic())
			handle_function(f, replace_intrinsics, indirect_funcs, indirect_check_proto);

	delete indirect_check_proto;

	modify_intrinsic_calls(m, replace_intrinsics);

	create_indirect_func_array(m, indirect_funcs);
	create_args_ptr(m);
	create_safestack_support(m);
}

struct GatePass: public ModulePass {
	static char ID;

	GatePass(): ModulePass(ID) {}

	bool runOnModule(Module &m) override
	{
		handle_module(m);
		return true;
	}
};

char GatePass::ID;

static RegisterPass<GatePass> x("gate", "Gate Pass", false, false);

} // namespace
