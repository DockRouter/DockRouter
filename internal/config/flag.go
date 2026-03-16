// Package config handles application configuration
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// FlagSet is a simple flag parser
type FlagSet struct {
	flags  map[string]*flag
	bools  map[string]*bool
	parsed bool
}

type flag struct {
	name  string
	value interface{}
	usage string
}

// NewFlagSet creates a new flag set
func NewFlagSet(name string) *FlagSet {
	return &FlagSet{
		flags: make(map[string]*flag),
		bools: make(map[string]*bool),
	}
}

// IntVar registers an int flag
func (f *FlagSet) IntVar(target *int, name string, value int, usage string) {
	*target = value
	f.flags[name] = &flag{name: name, value: target, usage: usage}
}

// StringVar registers a string flag
func (f *FlagSet) StringVar(target *string, name string, value string, usage string) {
	*target = value
	f.flags[name] = &flag{name: name, value: target, usage: usage}
}

// BoolVar registers a bool flag
func (f *FlagSet) BoolVar(target *bool, name string, value bool, usage string) {
	*target = value
	f.flags[name] = &flag{name: name, value: target, usage: usage}
	f.bools[name] = target
}

// DurationVar registers a duration flag
func (f *FlagSet) DurationVar(target *time.Duration, name string, value time.Duration, usage string) {
	*target = value
	f.flags[name] = &flag{name: name, value: target, usage: usage}
}

// StringSliceVar registers a string slice flag
func (f *FlagSet) StringSliceVar(target *[]string, name string, value []string, usage string) {
	*target = value
	f.flags[name] = &flag{name: name, value: target, usage: usage}
}

// Bool returns the value of a bool flag
func (f *FlagSet) Bool(name string) bool {
	if b, ok := f.bools[name]; ok {
		return *b
	}
	return false
}

// Parse parses the command line arguments
func (f *FlagSet) Parse(args []string) error {
	if f.parsed {
		return nil
	}
	f.parsed = true

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") {
			continue
		}

		// Normalize --flag to -flag
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			continue
		}

		// Check for = in the argument
		var value string
		if idx := strings.Index(name, "="); idx >= 0 {
			value = name[idx+1:]
			name = name[:idx]
		}

		flagDef, ok := f.flags[name]
		if !ok {
			return fmt.Errorf("unknown flag: %s", name)
		}

		// Handle bool flags
		if _, isBool := f.bools[name]; isBool {
			if value == "" {
				*(flagDef.value.(*bool)) = true
			} else {
				*(flagDef.value.(*bool)) = value == "true" || value == "1"
			}
			continue
		}

		// Get value from next arg if not in current arg
		if value == "" {
			if i+1 >= len(args) {
				return fmt.Errorf("flag %s requires a value", name)
			}
			i++
			value = args[i]
		}

		// Parse value based on type
		switch v := flagDef.value.(type) {
		case *int:
			parsed, err := parseInt(value)
			if err != nil {
				return fmt.Errorf("invalid value for %s: %v", name, err)
			}
			*v = parsed
		case *string:
			*v = value
		case *time.Duration:
			dur, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid duration for %s: %v", name, err)
			}
			*v = dur
		case *[]string:
			*v = strings.Split(value, ",")
		}
	}

	return nil
}

func parseInt(s string) (int, error) {
	var n int
	var negative bool
	if strings.HasPrefix(s, "-") {
		negative = true
		s = s[1:]
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		n = n*10 + int(c-'0')
	}
	if negative {
		n = -n
	}
	return n, nil
}

// PrintDefaults prints default values
func (f *FlagSet) PrintDefaults() {
	w := os.Stdout
	for name, fl := range f.flags {
		fmt.Fprintf(w, "  --%-15s %s\n", name, fl.usage)
	}
}
