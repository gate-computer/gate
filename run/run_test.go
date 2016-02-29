package run

import (
	"debug/elf"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	const memorySize = 256 * 1024 * 1024

	executorPath := os.Getenv("GATE_RUN_TEST_EXECUTOR")
	loaderPath := os.Getenv("GATE_RUN_TEST_LOADER")

	programPath := os.Getenv("GATE_RUN_TEST_ELF")
	if programPath == "" {
		t.Fatal("no test program")
	}

	program, err := elf.Open(programPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := Run(executorPath, loaderPath, program, memorySize); err != nil {
		t.Error(err)
	}
}

func TestBinaryUint64ToUint32Inplace(t *testing.T) {
	buf1 := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
	}

	t.Log(buf1)

	buf2 := binaryUint64ToUint32Inplace(buf1)

	t.Log(buf2)

	// TODO: check result
}
