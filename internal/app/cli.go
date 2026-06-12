package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zhulingtai/internal/buildinfo"
)

type RunOptions struct {
	Addr     string
	Headless bool
	PIDFile  string
}

func Execute(args []string) error {
	args, err := applyWorkdirFlag(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return Run()
	}

	if isHelpArg(args[0]) {
		printHelp()
		return nil
	}
	if isVersionArg(args[0]) {
		printVersion()
		return nil
	}

	switch args[0] {
	case "run":
		return runCommand(args[1:])
	case "start":
		return startCommand(args[1:])
	case "stop":
		return stopCommand(args[1:])
	case "restart":
		return restartCommand(args[1:])
	case "status":
		return statusCommand(args[1:])
	case "autostart":
		return autostartCommand(args[1:])
	case "version":
		printVersion()
		return nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("unknown flag: %s\n\n%s", args[0], helpText())
		}
		return fmt.Errorf("unknown command: %s\n\n%s", args[0], helpText())
	}
}

// applyWorkdirFlag strips a global "--workdir <path>" flag from args (used by
// autostart so the process starts in the directory recorded at enable time) and
// changes into it before any data files are touched.
func applyWorkdirFlag(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--workdir" || args[i] == "--work-dir" {
			if i+1 >= len(args) {
				return nil, errors.New("missing value for --workdir")
			}
			if err := os.Chdir(args[i+1]); err != nil {
				return nil, fmt.Errorf("chdir to workdir %q: %w", args[i+1], err)
			}
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out, nil
}

func runCommand(args []string) error {
	opts := DefaultRunOptions()
	opts.Headless = true
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--addr", "--listen":
			if i+1 >= len(args) {
				return errors.New("missing value for --addr")
			}
			opts.Addr = args[i+1]
			i++
		default:
			if isHelpArg(args[i]) {
				printHelp()
				return nil
			}
			return fmt.Errorf("unknown run argument: %s", args[i])
		}
	}
	return RunWithOptions(opts)
}

func startCommand(args []string) error {
	if hasHelpArg(args) {
		printHelp()
		return nil
	}
	opts, err := parseServiceCommandArgs(args)
	if err != nil {
		return err
	}

	if err := startDetached(opts.PIDFile, opts.Addr); err != nil {
		return err
	}
	return nil
}

func stopCommand(args []string) error {
	if hasHelpArg(args) {
		printHelp()
		return nil
	}
	opts, err := parseServiceCommandArgs(args)
	if err != nil {
		return err
	}
	return stopDetached(opts.PIDFile)
}

func restartCommand(args []string) error {
	if hasHelpArg(args) {
		printHelp()
		return nil
	}
	opts, err := parseServiceCommandArgs(args)
	if err != nil {
		return err
	}
	if err := stopDetached(opts.PIDFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return startDetached(opts.PIDFile, opts.Addr)
}

func statusCommand(args []string) error {
	if hasHelpArg(args) {
		printHelp()
		return nil
	}
	opts, err := parseServiceCommandArgs(args)
	if err != nil {
		return err
	}

	lock, err := readPIDFile(opts.PIDFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("stopped")
			return nil
		}
		return err
	}

	addr := lock.Addr
	if addr == "" {
		addr = defaultHTTPAddr
	}
	fmt.Printf("running pid=%d addr=%s\n", lock.PID, addr)
	return nil
}

func defaultPIDFile() string {
	return filepath.Join("data", "zlt.pid")
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func isVersionArg(arg string) bool {
	return arg == "-v" || arg == "--version"
}

func hasHelpArg(args []string) bool {
	for _, arg := range args {
		if isHelpArg(arg) {
			return true
		}
	}
	return false
}

func printVersion() {
	info := buildinfo.Current()
	fmt.Printf("%s %s\n\n", commandName(), buildinfo.DisplayVersion(info.Version))
	fmt.Printf("  Build timestamp:  %s\n", buildinfo.HumanBuildTime(info.BuildTime))
	fmt.Printf("  Git commit:       %s\n", info.Commit)
	fmt.Printf("  Build profile:    %s\n", info.BuildProfile)
	fmt.Printf("  Target platform:  %s\n", info.Platform)
	fmt.Printf("  Go compiler:      %s\n", info.GoVersion)
}

func commandName() string {
	return "zlt"
}

func printHelp() {
	fmt.Print(helpText())
}

func helpText() string {
	return `驻令台

用法:
  zlt
  zlt run [--addr <host:port>] [--workdir <path>]
  zlt start [--addr <host:port>] [--pid-file <path>]
  zlt stop [--pid-file <path>]
  zlt restart [--addr <host:port>] [--pid-file <path>]
  zlt status [--pid-file <path>]
  zlt autostart <enable|disable|status>
  zlt version
  zlt --version
  zlt -h | --help

说明:
  zlt
    默认启动图形/托盘模式

  zlt run
    以前台无界面模式运行

  zlt start
    以后端常驻方式启动

参数:
  --addr, --listen
    指定 HTTP 监听地址，例如:
    127.0.0.1:3719
    0.0.0.0:3719

  --pid-file
    指定后台进程状态文件路径
`
}

func parseServiceCommandArgs(args []string) (RunOptions, error) {
	opts := DefaultRunOptions()
	opts.Headless = true
	opts.PIDFile = defaultPIDFile()

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--addr", "--listen":
			if i+1 >= len(args) {
				return RunOptions{}, errors.New("missing value for --addr")
			}
			opts.Addr = args[i+1]
			i++
		case "--pid-file":
			if i+1 >= len(args) {
				return RunOptions{}, errors.New("missing value for --pid-file")
			}
			opts.PIDFile = args[i+1]
			i++
		default:
			return RunOptions{}, fmt.Errorf("unknown argument: %s", args[i])
		}
	}

	return opts, nil
}
