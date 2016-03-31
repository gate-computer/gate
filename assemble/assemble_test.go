package assemble

import (
	"bytes"
	"debug/elf"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
)

func TestAssemble(t *testing.T) {
	optBin := lookPath(t, os.Getenv("GATE_TEST_OPT"))
	passPlugin := os.Getenv("GATE_TEST_PASS_PLUGIN")
	llcBin := lookPath(t, os.Getenv("GATE_TEST_LLC"))
	asBin := lookPath(t, os.Getenv("GATE_TEST_AS"))
	ldBin := lookPath(t, os.Getenv("GATE_TEST_LD"))
	linkScript := os.Getenv("GATE_TEST_LINK_SCRIPT")
	bitcodePath := os.Getenv("GATE_TEST_BITCODE")

	bitcode, err := ioutil.ReadFile(bitcodePath)
	if err != nil {
		t.Fatal(err)
	}

	elfData, err := Assemble(optBin, passPlugin, llcBin, asBin, ldBin, linkScript, bitcode)
	if err != nil {
		t.Fatal(err)
	}

	if f, err := elf.NewFile(bytes.NewReader(elfData)); err != nil {
		t.Error(err)
	} else {
		f.Close()
	}
}

func lookPath(t *testing.T, name string) (path string) {
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return
}
