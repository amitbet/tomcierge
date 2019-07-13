package broadlinkrm

import (
	"encoding/json"
	"fmt"
)

type IRCommand []byte

func (i *IRCommand) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	var out []byte
	matched, err := fmt.Sscanf(s, "%x", &out)
	if err != nil {
		return err
	}

	if matched != 1 {
		return fmt.Errorf("invalid IR command format")
	}

	*i = out
	return nil
}

func (i IRCommand) MarshalJSON() ([]byte, error) {
	s := fmt.Sprintf("%x", []byte(i))
	return json.Marshal(s)
}
