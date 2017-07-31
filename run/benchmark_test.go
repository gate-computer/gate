package run_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/tsavola/gate/run"
	"github.com/tsavola/wag"
)

func readProgram(testName string) []byte {
	f := openProgram(testName)
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		panic(err)
	}

	return data
}

var (
	benchEnv = newEnvironment()

	benchProgNop   = readProgram("nop")
	benchProgHello = readProgram("hello")
	benchProgPeer  = readProgram("peer")
)

func BenchmarkLoadNop(b *testing.B)   { benchmarkLoad(b, benchProgNop) }
func BenchmarkLoadHello(b *testing.B) { benchmarkLoad(b, benchProgHello) }
func BenchmarkLoadPeer(b *testing.B)  { benchmarkLoad(b, benchProgPeer) }

func benchmarkLoad(b *testing.B, prog []byte) {
	for i := 0; i < b.N; i++ {
		m := wag.Module{
			MainSymbol: "main",
		}

		err := m.Load(bytes.NewReader(prog), benchEnv, new(bytes.Buffer), nil, run.RODataAddr, nil)
		if err != nil {
			panic(err)
		}
	}
}
