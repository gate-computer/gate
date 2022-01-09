// Code generated by gen.go, DO NOT EDIT!

package event

func (x *FailInternal) EventName() string         { return "FAIL_INTERNAL" }
func (x *FailNetwork) EventName() string          { return "FAIL_NETWORK" }
func (x *FailProtocol) EventName() string         { return "FAIL_PROTOCOL" }
func (x *FailRequest) EventName() string          { return "FAIL_REQUEST" }
func (x *IfaceAccess) EventName() string          { return "IFACE_ACCESS" }
func (x *InstanceConnect) EventName() string      { return "INSTANCE_CONNECT" }
func (x *InstanceCreateKnown) EventName() string  { return "INSTANCE_CREATE_KNOWN" }
func (x *InstanceCreateStream) EventName() string { return "INSTANCE_CREATE_STREAM" }
func (x *InstanceDebug) EventName() string        { return "INSTANCE_DEBUG" }
func (x *InstanceDelete) EventName() string       { return "INSTANCE_DELETE" }
func (x *InstanceDisconnect) EventName() string   { return "INSTANCE_DISCONNECT" }
func (x *InstanceInfo) EventName() string         { return "INSTANCE_INFO" }
func (x *InstanceKill) EventName() string         { return "INSTANCE_KILL" }
func (x *InstanceList) EventName() string         { return "INSTANCE_LIST" }
func (x *InstanceResume) EventName() string       { return "INSTANCE_RESUME" }
func (x *InstanceSnapshot) EventName() string     { return "INSTANCE_SNAPSHOT" }
func (x *InstanceStop) EventName() string         { return "INSTANCE_STOP" }
func (x *InstanceSuspend) EventName() string      { return "INSTANCE_SUSPEND" }
func (x *InstanceUpdate) EventName() string       { return "INSTANCE_UPDATE" }
func (x *InstanceWait) EventName() string         { return "INSTANCE_WAIT" }
func (x *ModuleDownload) EventName() string       { return "MODULE_DOWNLOAD" }
func (x *ModuleInfo) EventName() string           { return "MODULE_INFO" }
func (x *ModuleList) EventName() string           { return "MODULE_LIST" }
func (x *ModulePin) EventName() string            { return "MODULE_PIN" }
func (x *ModuleSourceExist) EventName() string    { return "MODULE_SOURCE_EXIST" }
func (x *ModuleSourceNew) EventName() string      { return "MODULE_SOURCE_NEW" }
func (x *ModuleUnpin) EventName() string          { return "MODULE_UNPIN" }
func (x *ModuleUploadExist) EventName() string    { return "MODULE_UPLOAD_EXIST" }
func (x *ModuleUploadNew) EventName() string      { return "MODULE_UPLOAD_NEW" }

func (*FailInternal) EventType() int32         { return int32(Type_FAIL_INTERNAL) }
func (*FailNetwork) EventType() int32          { return int32(Type_FAIL_NETWORK) }
func (*FailProtocol) EventType() int32         { return int32(Type_FAIL_PROTOCOL) }
func (*FailRequest) EventType() int32          { return int32(Type_FAIL_REQUEST) }
func (*IfaceAccess) EventType() int32          { return int32(Type_IFACE_ACCESS) }
func (*InstanceConnect) EventType() int32      { return int32(Type_INSTANCE_CONNECT) }
func (*InstanceCreateKnown) EventType() int32  { return int32(Type_INSTANCE_CREATE_KNOWN) }
func (*InstanceCreateStream) EventType() int32 { return int32(Type_INSTANCE_CREATE_STREAM) }
func (*InstanceDebug) EventType() int32        { return int32(Type_INSTANCE_DEBUG) }
func (*InstanceDelete) EventType() int32       { return int32(Type_INSTANCE_DELETE) }
func (*InstanceDisconnect) EventType() int32   { return int32(Type_INSTANCE_DISCONNECT) }
func (*InstanceInfo) EventType() int32         { return int32(Type_INSTANCE_INFO) }
func (*InstanceKill) EventType() int32         { return int32(Type_INSTANCE_KILL) }
func (*InstanceList) EventType() int32         { return int32(Type_INSTANCE_LIST) }
func (*InstanceResume) EventType() int32       { return int32(Type_INSTANCE_RESUME) }
func (*InstanceSnapshot) EventType() int32     { return int32(Type_INSTANCE_SNAPSHOT) }
func (*InstanceStop) EventType() int32         { return int32(Type_INSTANCE_STOP) }
func (*InstanceSuspend) EventType() int32      { return int32(Type_INSTANCE_SUSPEND) }
func (*InstanceUpdate) EventType() int32       { return int32(Type_INSTANCE_UPDATE) }
func (*InstanceWait) EventType() int32         { return int32(Type_INSTANCE_WAIT) }
func (*ModuleDownload) EventType() int32       { return int32(Type_MODULE_DOWNLOAD) }
func (*ModuleInfo) EventType() int32           { return int32(Type_MODULE_INFO) }
func (*ModuleList) EventType() int32           { return int32(Type_MODULE_LIST) }
func (*ModulePin) EventType() int32            { return int32(Type_MODULE_PIN) }
func (*ModuleSourceExist) EventType() int32    { return int32(Type_MODULE_SOURCE_EXIST) }
func (*ModuleSourceNew) EventType() int32      { return int32(Type_MODULE_SOURCE_NEW) }
func (*ModuleUnpin) EventType() int32          { return int32(Type_MODULE_UNPIN) }
func (*ModuleUploadExist) EventType() int32    { return int32(Type_MODULE_UPLOAD_EXIST) }
func (*ModuleUploadNew) EventType() int32      { return int32(Type_MODULE_UPLOAD_NEW) }
