package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"zhulingtai/internal/auth"
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
	case "auth":
		return authCommand(args[1:])
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
		case "--pid-file":
			// Used by the detached daemon so the child owns the single-instance
			// lock (and so `status`/`stop` can find it). Foreground runs may set
			// it too; an empty value falls back to the default in RunWithOptions.
			if i+1 >= len(args) {
				return errors.New("missing value for --pid-file")
			}
			opts.PIDFile = args[i+1]
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

	// A pid file can linger after an unclean exit; only report "running" when the
	// recorded process is actually alive and still us.
	if !processMatches(lock.PID, lock.Exe) {
		fmt.Println("stopped")
		return nil
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
  zlt run [--addr <host:port>] [--pid-file <path>] [--workdir <path>]
  zlt start [--addr <host:port>] [--pid-file <path>]
  zlt stop [--pid-file <path>]
  zlt restart [--addr <host:port>] [--pid-file <path>]
  zlt status [--pid-file <path>]
  zlt autostart <enable|disable|status>
  zlt auth <show|reset> [--pid-file <path>]
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

  单例运行:
    同一工作目录下仅允许一个实例运行。重复启动时，托盘模式会打开已运行
    实例的控制面板，命令行模式会报错退出。

参数:
  --addr, --listen
    指定 HTTP 监听地址，例如:
    127.0.0.1:3719
    0.0.0.0:3719

  --pid-file
    指定进程状态/单例锁文件路径 (默认 data/zlt.pid)
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

// authCommand implements `zlt auth show` and `zlt auth reset`.
//
//   show   — prints the base64url access key stored in data/auth.key, so the
//            operator can paste it into the browser login. Works whether or not
//            the service is running.
//   reset  — deletes the key file and the session database so the next start
//            generates a fresh key. Requires the instance to be stopped (a
//            running instance would just rewrite the old key on its next
//            restart anyway, and deleting under it leaves stale sessions).
//            Supports --pid-file to target a non-default instance.
func authCommand(args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		fmt.Print(authHelpText())
		return nil
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "show":
		return authShowCommand(rest)
	case "reset":
		return authResetCommand(rest)
	default:
		return fmt.Errorf("unknown auth subcommand: %s\n\n%s", sub, authHelpText())
	}
}

func authShowCommand(args []string) error {
	for _, a := range args {
		if isHelpArg(a) {
			fmt.Print(authHelpText())
			return nil
		}
		return fmt.Errorf("unknown argument: %s", a)
	}
	keyPath := filepath.Join("data", "auth.key")
	display, err := auth.ReadKeyFile(keyPath)
	if err != nil {
		return err
	}
	fmt.Println(display)
	return nil
}

func authResetCommand(args []string) error {
	pidFile := defaultPIDFile()
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--pid-file":
			if i+1 >= len(args) {
				return errors.New("missing value for --pid-file")
			}
			pidFile = args[i+1]
			i++
		default:
			if isHelpArg(args[i]) {
				fmt.Print(authHelpText())
				return nil
			}
			return fmt.Errorf("unknown argument: %s", args[i])
		}
	}

	// The instance must be stopped: resetting the key while it is running would
	// leave the process holding the old key in memory and all logged-in
	// browsers unable to tell until a restart.
	if lock, err := readPIDFile(pidFile); err == nil {
		if processMatches(lock.PID, lock.Exe) {
			return fmt.Errorf("驻令台 正在运行 (pid %d)，请先停止后再重置密钥", lock.PID)
		}
		_ = os.Remove(pidFile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	keyPath := filepath.Join("data", "auth.key")
	if err := os.Remove(keyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The session database is separate so dropping it does not touch task data.
	if err := os.Remove(filepath.Join("data", "auth.db")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("访问密钥已重置。下次启动将生成新密钥，所有已登录浏览器需要重新登录。")
	return nil
}

func authHelpText() string {
	return `zlt auth — 访问密钥管理

用法:
  zlt auth show
    打印 data/auth.key 中保存的访问密钥（浏览器登录时输入）

  zlt auth reset [--pid-file <path>]
    删除密钥文件和会话数据库，下次启动生成新密钥
    要求对应实例已停止

参数:
  --pid-file
    指定进程状态/单例锁文件路径 (默认 data/zlt.pid)
`
}
