package models

import (
	"fmt"
	"sort"

	"kvraft/api"

	"github.com/anishathalye/porcupine"
)

type KvInput struct {
	Op      uint8 // 0 => get, 1 => put
	Key     string
	Value   string
	Version uint64
}

type KvOutput struct {
	Value   string
	Version uint64
	Err     string
}

type KvState struct {
	Value   string
	Version uint64
}

var KvModel = porcupine.Model{
	Partition: func(history []porcupine.Operation) [][]porcupine.Operation {
		m := make(map[string][]porcupine.Operation)
		for _, v := range history {
			key := v.Input.(KvInput).Key
			m[key] = append(m[key], v)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		ret := make([][]porcupine.Operation, 0, len(keys))
		for _, k := range keys {
			ret = append(ret, m[k])
		}
		return ret
	},
	Init: func() interface{} {
		// note: we are modeling a single key's value here;
		// we're partitioning by key, so this is okay
		return KvState{Value: "", Version: 0}
	},
	Step: func(state, input, output interface{}) (bool, interface{}) {
		inp := input.(KvInput)
		out := output.(KvOutput)
		st := state.(KvState)

		switch inp.Op {
		case 0: // get
			if out.Err == string(api.OK) {
				return out.Value == st.Value, st
			}
			return true, st
		case 1: // put
			if out.Err == string(api.OK) {
				if st.Version == inp.Version {
					return true, KvState{Value: inp.Value, Version: st.Version + 1}
				}
				return false, st
			}

			if out.Err == string(api.ErrMaybe) {
				if st.Version == inp.Version {
					return true, KvState{Value: inp.Value, Version: st.Version + 1}
				}
				return true, st
			}

			return true, st
		default:
			return false, st
		}
	},
	DescribeOperation: func(input, output interface{}) string {
		inp := input.(KvInput)
		out := output.(KvOutput)
		switch inp.Op {
		case 0:
			return fmt.Sprintf("get('%s') -> ('%s', v%d, %s)", inp.Key, out.Value, out.Version, out.Err)
		case 1:
			return fmt.Sprintf("put('%s', '%s', v%d) -> %s", inp.Key, inp.Value, inp.Version, out.Err)
		default:
			return "<invalid>"
		}
	},
}
