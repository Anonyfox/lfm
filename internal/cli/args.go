package cli

import (
	"errors"
	"fmt"
	"strings"
)

// flagSpec declares one --flag. Boolean flags take no value; value flags read
// "--name=value" or the next argument.
type flagSpec struct {
	name  string
	value bool
	usage string
}

type parsed struct {
	flags map[string]string
	args  []string
	help  bool
}

func (p parsed) has(name string) bool { _, ok := p.flags[name]; return ok }

// errUsage marks mistakes in the command line itself (exit code 2).
var errUsage = errors.New("usage")

func usageError(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{errUsage}, args...)...)
}

// parse reads flags anywhere among the arguments, so `lfm compile --release
// site` and `lfm compile site --release` mean the same. "--" ends flags.
func parse(command string, args []string, specs []flagSpec) (parsed, error) {
	p := parsed{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			p.args = append(p.args, args[i+1:]...)
			break
		}
		if arg == "-h" || arg == "--help" {
			p.help = true
			continue
		}
		if !strings.HasPrefix(arg, "--") || arg == "--" {
			if strings.HasPrefix(arg, "-") && arg != "-" {
				return p, usageError("unknown flag %s for lfm %s; flags are spelled --name", arg, command)
			}
			p.args = append(p.args, arg)
			continue
		}
		name, value, hasValue := strings.Cut(arg[2:], "=")
		spec, ok := lookup(specs, name)
		if !ok {
			return p, usageError("unknown flag --%s for lfm %s", name, command)
		}
		if _, repeated := p.flags[name]; repeated {
			return p, usageError("--%s given twice", name)
		}
		switch {
		case !spec.value && hasValue:
			return p, usageError("--%s takes no value", name)
		case spec.value && !hasValue:
			if i+1 >= len(args) {
				return p, usageError("--%s needs a value", name)
			}
			i++
			value = args[i]
		}
		p.flags[name] = value
	}
	return p, nil
}

func lookup(specs []flagSpec, name string) (flagSpec, bool) {
	for _, s := range specs {
		if s.name == name {
			return s, true
		}
	}
	return flagSpec{}, false
}

// directory returns the single optional positional argument, defaulting to ".".
func (p parsed) directory(command string) (string, error) {
	switch len(p.args) {
	case 0:
		return ".", nil
	case 1:
		return p.args[0], nil
	default:
		return "", usageError("lfm %s takes one directory, got %d arguments", command, len(p.args))
	}
}
