package broadlinkrm

import (
	"fmt"
	"sort"
)

type Remote struct {
	Name     string               `json:"name"`
	Commands map[string]IRCommand `json:"commands"`
}

type RemoteList []*Remote

func NewRemote(name string) *Remote {
	return &Remote{
		Name:     name,
		Commands: make(map[string]IRCommand),
	}
}

func (r *Remote) AddCommand(name string, irCode []byte) error {
	_, ok := r.Commands[name]
	if ok {
		return fmt.Errorf("command %s already exists", name)
	}

	cmd := make([]byte, len(irCode))
	copy(cmd, irCode)
	r.Commands[name] = cmd
	return nil
}

func (r *Remote) CommandNames() []string {
	out := make([]string, 0, len(r.Commands))
	for k := range r.Commands {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (rl RemoteList) Find(remoteName string) *Remote {
	for _, r := range rl {
		if r.Name == remoteName {
			return r
		}
	}
	return nil
}

func (rl RemoteList) Names() []string {
	out := make([]string, len(rl))
	for idx, r := range rl {
		out[idx] = r.Name
	}
	return out
}
