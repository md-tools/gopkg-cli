package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

// ToSnakeCase conver string to snake case
func ToSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}-${2}")
	return strings.ToLower(snake)
}

type optState int

//OptStates
const (
	OptStateUntouched optState = iota
	OptStateFlagPassed
	OptStateEnvPassed
)

// Opt helps gather value from cli flag or env var
type Opt struct {
	Name        string
	Description string
	Value       string
	State       optState
	SetFunc     func(string)
	Required    bool
}

func (opt *Opt) String() string {
	return opt.Value
}

// Set value during 'flag.Parse'
func (opt *Opt) Set(flagval string) error {
	opt.SetFunc(flagval)
	opt.Value = flagval
	opt.State = OptStateFlagPassed
	return nil
}

// Flags container for cmd flags
type Flags map[string]*Opt

// Cmd handles cli commands and subcommands
type Cmd struct {
	ExecutedAs string
	Name       string
	Args       []string
	Opts       interface{}
	SubCmds    []Cmd
	ParsedOpts []*Opt
	Run        func() error
}

// AddSubCmd ...
func (cmd Cmd) AddSubCmd(subcmd Cmd) {
	cmd.SubCmds = append(cmd.SubCmds, subcmd)
}

type reflected struct {
	Type  reflect.Type
	Field reflect.StructField
	Value reflect.Value
}

func structUnwrap(r *reflected) []*reflected {
	fields := []*reflected{}
	for fieldIndex := 0; fieldIndex < r.Type.NumField(); fieldIndex++ {
		item := &reflected{
			Type:  r.Type.Field(fieldIndex).Type,
			Field: r.Type.Field(fieldIndex),
			Value: r.Value.Field(fieldIndex),
		}
		if item.Field.Type.Kind() == reflect.Struct {
			fields = append(fields, structUnwrap(item)...)
			continue
		}
		fields = append(fields, item)
	}
	return fields
}

func reflectStruct(i interface{}) []*reflected {
	return structUnwrap(&reflected{
		Type:  reflect.TypeOf(i).Elem(),
		Value: reflect.ValueOf(i).Elem(),
	})
}

// Init bind and parse flags
func (cmd Cmd) Init(args []string) error {
	if cmd.Opts == nil {
		return nil
	}
	flags := flag.NewFlagSet(cmd.Name, flag.ExitOnError)
	for _, ref := range reflectStruct(cmd.Opts) {
		opt := &Opt{
			Name:        ToSnakeCase(ref.Field.Name),
			Description: ref.Field.Tag.Get("desc"),
			SetFunc:     ref.Value.SetString,
			Required:    false,
		}
		if requiredTag, ok := ref.Field.Tag.Lookup("required"); ok {
			required, err := strconv.ParseBool(requiredTag)
			if err != nil {
				return err
			}
			opt.Required = required
		}
		cmd.ParsedOpts = append(cmd.ParsedOpts, opt)
		flags.Var(opt, opt.Name, opt.Description)
	}
	flags.Parse(args)
	log.Printf("remaining args: %v\n", flags.Args())
	for _, opt := range cmd.ParsedOpts {
		if opt.State != OptStateFlagPassed {
			if envval, ok := os.LookupEnv(strings.ToUpper(opt.Name)); ok {
				opt.SetFunc(envval)
				opt.Value = envval
				opt.State = OptStateEnvPassed
			}
		}
		if opt.State == OptStateUntouched && opt.Required {
			return fmt.Errorf("opt '%s' is required but not passed", opt.Name)
		}
	}
	return nil
}

// SubCmd get sub cmd by name
func (cmd Cmd) SubCmd(subcmdname string) (*Cmd, bool) {
	for _, subcmd := range cmd.SubCmds {
		if subcmd.Name == subcmdname {
			return &subcmd, true
		}
	}
	return nil, false
}

// Execute given command
func (cmd Cmd) Execute() error {
	cmd.ExecutedAs = os.Args[0]
	targetCmd := &cmd
	args := os.Args[1:]
	for len(targetCmd.SubCmds) > 0 {
		if len(args) < 1 {
			return fmt.Errorf("%s: not engough arguments", targetCmd.Name)
		}
		subCmdName := args[0]
		subCmd, ok := targetCmd.SubCmd(subCmdName)
		if !ok {
			return fmt.Errorf("'%s' is not a %s command", subCmdName, targetCmd.Name)
		}
		targetCmd = subCmd
		args = args[1:]
	}
	if err := targetCmd.Init(args); err != nil {
		return err
	}
	return targetCmd.Run()
}
