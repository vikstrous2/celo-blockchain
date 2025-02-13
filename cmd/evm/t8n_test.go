package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/celo-org/celo-blockchain/internal/cmdtest"
	"github.com/docker/docker/pkg/reexec"
)

func TestMain(m *testing.M) {
	// Run the app if we've been exec'd as "ethkey-test" in runEthkey.
	reexec.Register("evm-test", func() {
		if err := app.Run(os.Args); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	})
	// check if we have been reexec'd
	if reexec.Init() {
		return
	}
	os.Exit(m.Run())
}

type testT8n struct {
	*cmdtest.TestCmd
}

type t8nInput struct {
	inAlloc  string
	inTxs    string
	inEnv    string
	stFork   string
	stReward string
}

func (args *t8nInput) get(base string) []string {
	var out []string
	if opt := args.inAlloc; opt != "" {
		out = append(out, "--input.alloc")
		out = append(out, fmt.Sprintf("%v/%v", base, opt))
	}
	if opt := args.inTxs; opt != "" {
		out = append(out, "--input.txs")
		out = append(out, fmt.Sprintf("%v/%v", base, opt))
	}
	if opt := args.inEnv; opt != "" {
		out = append(out, "--input.env")
		out = append(out, fmt.Sprintf("%v/%v", base, opt))
	}
	if opt := args.stFork; opt != "" {
		out = append(out, "--state.fork", opt)
	}
	if opt := args.stReward; opt != "" {
		out = append(out, "--state.reward", opt)
	}
	return out
}

type t8nOutput struct {
	alloc  bool
	result bool
	body   bool
}

func (args *t8nOutput) get() (out []string) {
	out = append(out, "t8n")
	if args.body {
		out = append(out, "--output.body", "stdout")
	} else {
		out = append(out, "--output.body", "") // empty means ignore
	}
	if args.result {
		out = append(out, "--output.result", "stdout")
	} else {
		out = append(out, "--output.result", "")
	}
	if args.alloc {
		out = append(out, "--output.alloc", "stdout")
	} else {
		out = append(out, "--output.alloc", "")
	}
	return out
}

func TestT8n(t *testing.T) {
	t.Skip() // TODO: T8n not yet ready in CELO
	tt := new(testT8n)
	tt.TestCmd = cmdtest.NewTestCmd(t, tt)
	for i, tc := range []struct {
		base        string
		input       t8nInput
		output      t8nOutput
		expExitCode int
		expOut      string
	}{
		{ // Test exit (3) on bad config
			base: "./testdata/1",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "Frontier+1346", "",
			},
			output:      t8nOutput{alloc: true, result: true},
			expExitCode: 3,
		},
		{
			base: "./testdata/1",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "Byzantium", "",
			},
			output: t8nOutput{alloc: true, result: true},
			expOut: "exp.json",
		},
		{ // blockhash test
			base: "./testdata/3",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "Berlin", "",
			},
			output: t8nOutput{alloc: true, result: true},
			expOut: "exp.json",
		},
		{ // missing blockhash test
			base: "./testdata/4",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "Berlin", "",
			},
			output:      t8nOutput{alloc: true, result: true},
			expExitCode: 4,
		},
		{ // Ommer test
			base: "./testdata/5",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "Byzantium", "0x80",
			},
			output: t8nOutput{alloc: true, result: true},
			expOut: "exp.json",
		},
		{ // Sign json transactions
			base: "./testdata/13",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "London", "",
			},
			output: t8nOutput{body: true},
			expOut: "exp.json",
		},
		{ // Already signed transactions
			base: "./testdata/13",
			input: t8nInput{
				"alloc.json", "signed_txs.rlp", "env.json", "London", "",
			},
			output: t8nOutput{result: true},
			expOut: "exp2.json",
		},
		{ // Difficulty calculation - no uncles
			base: "./testdata/14",
			input: t8nInput{
				"alloc.json", "txs.json", "env.json", "London", "",
			},
			output: t8nOutput{result: true},
			expOut: "exp.json",
		},
		{ // Difficulty calculation - with uncles
			base: "./testdata/14",
			input: t8nInput{
				"alloc.json", "txs.json", "env.uncles.json", "London", "",
			},
			output: t8nOutput{result: true},
			expOut: "exp2.json",
		},
	} {

		args := append(tc.output.get(), tc.input.get(tc.base)...)
		tt.Run("evm-test", args...)
		tt.Logf("args: %v\n", strings.Join(args, " "))
		// Compare the expected output, if provided
		if tc.expOut != "" {
			want, err := os.ReadFile(fmt.Sprintf("%v/%v", tc.base, tc.expOut))
			if err != nil {
				t.Fatalf("test %d: could not read expected output: %v", i, err)
			}
			have := tt.Output()
			ok, err := cmpJson(have, want)
			switch {
			case err != nil:
				t.Fatalf("test %d, json parsing failed: %v", i, err)
			case !ok:
				t.Fatalf("test %d: output wrong, have \n%v\nwant\n%v\n", i, string(have), string(want))
			}
		}
		tt.WaitExit()
		if have, want := tt.ExitStatus(), tc.expExitCode; have != want {
			t.Fatalf("test %d: wrong exit code, have %d, want %d", i, have, want)
		}
	}
}

// cmpJson compares the JSON in two byte slices.
func cmpJson(a, b []byte) (bool, error) {
	var j, j2 interface{}
	if err := json.Unmarshal(a, &j); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &j2); err != nil {
		return false, err
	}
	return reflect.DeepEqual(j2, j), nil
}
