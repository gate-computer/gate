package run

import (
	"debug/elf"
	"os"
	"testing"
)

func TestRun(t *testing.T) {
	const memorySize = 256 * 1024 * 1024

	executorBin := os.Getenv("GATE_TEST_EXECUTOR")
	loaderBin := os.Getenv("GATE_TEST_LOADER")
	elfPath := os.Getenv("GATE_TEST_ELF")

	elfFile, err := elf.Open(elfPath)
	if err != nil {
		t.Fatal(err)
	}

	payload, err := NewPayload(elfFile, memorySize)
	if err != nil {
		t.Fatal(err)
	}

	err = Run(executorBin, loaderBin, payload)
	if err != nil {
		t.Fatal(err)
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
