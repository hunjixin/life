package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/perlin-network/life/exec"
)

type Resolver struct{}

func (r *Resolver) ResolveFunc(module, field string) exec.FunctionImport {
	if module != "spectest" {
		panic("module != spectest")
	}
	if field != "print_i32" {
		panic(fmt.Errorf("func %s not found", field))
	}
	return func(vm *exec.VirtualMachine) int64 { return 0 }
}

func (r *Resolver) ResolveGlobal(module, field string) int64 {
	if module != "spectest" {
		panic("module != spectest")
	}
	if field != "global_i32" {
		panic(fmt.Errorf("global %s not found", field))
	}
	return 0
}

type Config struct {
	SourceFilename string    `json:"source_filename"`
	Commands       []Command `json:"commands"`
}

type Command struct {
	Type       string      `json:"type"`
	Line       int         `json:"line"`
	Filename   string      `json:"filename"`
	Name       string      `json:"name"`
	Action     CmdAction   `json:"action"`
	Text       string      `json:"text"`
	ModuleType string      `json:"module_type"`
	Expected   []ValueInfo `json:"expected"`
}

type CmdAction struct {
	Type     string      `json:"type"`
	Module   string      `json:"module"`
	Field    string      `json:"field"`
	Args     []ValueInfo `json:"args"`
	Expected []ValueInfo `json:"expected"`
}

type ValueInfo struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func LoadConfigFromFile(filename string) *Config {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	var cfg Config
	err = json.Unmarshal(raw, &cfg)
	if err != nil {
		panic(err)
	}
	return &cfg
}

func (c *Config) Run(cfgPath string) error {
	var vm *exec.VirtualMachine
	namedVMs := make(map[string]*exec.VirtualMachine)

	dir, _ := filepath.Split(cfgPath)

	for _, cmd := range c.Commands {
		switch cmd.Type {
		case "module":
			input, err := ioutil.ReadFile(path.Join(dir, cmd.Filename))
			if err != nil {
				panic(err)
			}
			localVM, err := exec.NewVirtualMachine(input, exec.VMConfig{
				EnableJIT:      true,
				MaxMemoryPages: 1024, // for memory trap tests
			}, &Resolver{})
			if err != nil {
				panic(err)
			}
			vm = localVM
			if cmd.Name != "" {
				namedVMs[cmd.Name] = localVM
			}
		case "assert_return", "action":
			localVM := vm
			if cmd.Action.Module != "" {
				if target, ok := namedVMs[cmd.Action.Module]; ok {
					localVM = target
				} else {
					panic("named module not found")
				}
			}

			switch cmd.Action.Type {
			case "invoke":
				entryID, ok := localVM.GetFunctionExport(cmd.Action.Field)
				if !ok {
					panic("export not found (func)")
				}
				args := make([]int64, 0)
				for _, arg := range cmd.Action.Args {
					var val uint64
					fmt.Sscanf(arg.Value, "%d", &val)
					args = append(args, int64(val))
				}
				fmt.Printf("Entry = %d\n", entryID)
				ret, err := localVM.Run(entryID, args...)
				if err != nil {
					panic(err)
				}
				if len(cmd.Action.Expected) != 0 {
					var _exp uint64
					fmt.Sscanf(cmd.Action.Expected[0].Value, "%d", &_exp)
					exp := int64(_exp)
					if cmd.Action.Expected[0].Type == "i32" || cmd.Action.Expected[0].Type == "f32" {
						ret = int64(uint32(ret))
						exp = int64(uint32(exp))
					}
					if ret != exp {
						panic(fmt.Errorf("ret mismatch: got %d, expected %d\n", ret, exp))
					}
				}
			case "get":
				globalID, ok := localVM.GetGlobalExport(cmd.Action.Field)
				if !ok {
					panic("export not found (global)")
				}
				val := localVM.Globals[globalID]
				var _exp uint64
				fmt.Sscanf(cmd.Expected[0].Value, "%d", &_exp)
				exp := int64(_exp)
				if cmd.Expected[0].Type == "i32" || cmd.Expected[0].Type == "f32" {
					val = int64(uint32(val))
					exp = int64(uint32(exp))
				}
				if val != exp {
					panic(fmt.Errorf("val mismatch: got %d, expected %d\n", val, exp))
				}
			default:
				panic(cmd.Action.Type)
			}
		case "assert_trap", "assert_malformed", "assert_invalid", "assert_exhaustion", "assert_unlinkable",
			"assert_return_canonical_nan", "assert_return_arithmetic_nan":
			fmt.Printf("skipping %s\n", cmd.Type)
		default:
			panic(cmd.Type)
		}
		fmt.Printf("PASS L%d\n", cmd.Line)
	}

	return nil
}

func main() {
	cfg := LoadConfigFromFile(os.Args[1])
	err := cfg.Run(os.Args[1])
	if err != nil {
		panic(err)
	}
}
