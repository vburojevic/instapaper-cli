package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/vburojevic/instapaper-cli/internal/browser"
	"github.com/vburojevic/instapaper-cli/internal/config"
	"github.com/vburojevic/instapaper-cli/internal/instapaper"
	"github.com/vburojevic/instapaper-cli/internal/oauth1"
	"github.com/vburojevic/instapaper-cli/internal/output"
	"github.com/vburojevic/instapaper-cli/internal/prompt"
	"github.com/vburojevic/instapaper-cli/internal/version"
)

type GlobalOptions struct {
	ConfigPath   string
	Format       string
	Quiet        bool
	Verbose      bool
	Debug        bool
	DebugJSON    bool
	Timeout      time.Duration
	APIBase      string
	OutputPath   string
	StderrJSON   bool
	RetryCount   int
	RetryBackoff time.Duration
	DryRun       bool
	Idempotent   bool
}

var stderrJSONEnabled bool

type durationFlag struct {
	value *time.Duration
	set   bool
}

func (d *durationFlag) String() string {
	if d == nil || d.value == nil {
		return ""
	}
	return d.value.String()
}

func (d *durationFlag) Set(s string) error {
	if d == nil || d.value == nil {
		return errors.New("duration flag not initialized")
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d.value = v
	d.set = true
	return nil
}

func main() {
	os.Exit(run(os.Args, os.Stdout, os.Stderr))
}

func run(argv []string, stdout, stderr io.Writer) int {
	global := flag.NewFlagSet("ip", flag.ContinueOnError)
	global.SetOutput(stderr)
	var opts GlobalOptions
	var help bool
	var showVersion bool
	var jsonOutput bool
	var plainOutput bool
	var ndjsonOutput bool
	var jsonlOutput bool
	var timeoutFlag durationFlag
	opts.Timeout = 15 * time.Second
	timeoutFlag.value = &opts.Timeout
	global.StringVar(&opts.ConfigPath, "config", "", "Path to config file (default: user config dir)")
	global.StringVar(&opts.Format, "format", "", "Output format: table, plain, json, or ndjson")
	global.BoolVar(&opts.Quiet, "quiet", false, "Less output")
	global.BoolVar(&opts.Verbose, "verbose", false, "More output")
	global.BoolVar(&opts.Debug, "debug", false, "Debug output (never prints secrets)")
	global.BoolVar(&opts.DebugJSON, "debug-json", false, "Debug output as JSON lines")
	global.Var(&timeoutFlag, "timeout", "HTTP timeout")
	global.StringVar(&opts.APIBase, "api-base", "", "API base URL (default: https://www.instapaper.com)")
	global.BoolVar(&jsonOutput, "json", false, "Output JSON (alias for --format json)")
	global.BoolVar(&plainOutput, "plain", false, "Output plain text (alias for --format plain)")
	global.BoolVar(&ndjsonOutput, "ndjson", false, "Output NDJSON (alias for --format ndjson)")
	global.BoolVar(&jsonlOutput, "jsonl", false, "Output NDJSON (alias for --format ndjson)")
	global.StringVar(&opts.OutputPath, "output", "", "Write output to file ('-' for stdout)")
	global.BoolVar(&opts.StderrJSON, "stderr-json", false, "Emit errors as JSON on stderr")
	global.IntVar(&opts.RetryCount, "retry", 0, "Retry count for transient errors")
	global.DurationVar(&opts.RetryBackoff, "retry-backoff", 500*time.Millisecond, "Retry backoff base duration")
	global.BoolVar(&opts.DryRun, "dry-run", false, "Preview actions without making changes")
	global.BoolVar(&opts.Idempotent, "idempotent", false, "Ignore already-in-state errors when possible")
	global.BoolVar(&showVersion, "version", false, "Show version")
	global.BoolVar(&help, "help", false, "Show help")
	global.BoolVar(&help, "h", false, "Show help")
	global.Usage = func() { fmt.Fprintln(stderr, usageRoot()) }

	if err := global.Parse(argv[1:]); err != nil {
		return 2
	}
	if !timeoutFlag.set {
		if env := os.Getenv("INSTAPAPER_TIMEOUT"); env != "" {
			d, err := time.ParseDuration(env)
			if err != nil {
				return printUsageError(stderr, fmt.Sprintf("invalid INSTAPAPER_TIMEOUT: %v", err))
			}
			opts.Timeout = d
		}
	}
	if opts.DebugJSON {
		opts.Debug = true
	}
	stderrJSONEnabled = opts.StderrJSON
	if help {
		fmt.Fprintln(stdout, usageRoot())
		return 0
	}
	if showVersion {
		fmt.Fprintln(stdout, version.String())
		return 0
	}
	args := global.Args()
	if len(args) == 0 {
		fmt.Fprintln(stderr, usageRoot())
		return 2
	}

	cfgPath, err := resolveConfigPath(opts.ConfigPath)
	if err != nil {
		return printError(stderr, err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return printError(stderr, err)
	}

	// Resolve base URL
	if opts.APIBase == "" {
		if env := os.Getenv("INSTAPAPER_API_BASE"); env != "" {
			opts.APIBase = env
		} else if cfg.APIBase != "" {
			opts.APIBase = cfg.APIBase
		} else {
			opts.APIBase = config.DefaultBaseURL()
		}
	}

	// Resolve default output format
	if opts.Format == "" {
		if cfg.Defaults.Format != "" {
			opts.Format = cfg.Defaults.Format
		} else {
			opts.Format = "ndjson"
		}
	}
	if jsonOutput {
		opts.Format = "json"
	}
	if plainOutput {
		opts.Format = "plain"
	}
	if ndjsonOutput || jsonlOutput {
		opts.Format = "ndjson"
	}
	if err := validateFormat(opts.Format); err != nil {
		return printUsageError(stderr, err.Error())
	}

	if opts.OutputPath != "" {
		out, closeFn, err := openOutputWriter(opts.OutputPath, stdout)
		if err != nil {
			return printError(stderr, err)
		}
		if closeFn != nil {
			defer closeFn()
		}
		stdout = out
	}

	ctx := context.Background()
	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "help", "-h", "--help":
		return runHelp(cmdArgs, stdout, stderr)
	case "version":
		fmt.Fprintln(stdout, version.String())
		return 0
	case "config":
		return runConfig(cmdArgs, cfgPath, &opts, stdout, stderr)
	case "auth":
		return runAuth(ctx, cmdArgs, &opts, cfg, cfgPath, stdout, stderr)
	case "add":
		return runAdd(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "list":
		return runList(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "export":
		return runExport(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "import":
		return runImport(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "progress":
		return runProgress(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "archive", "unarchive", "star", "unstar":
		return runBookmarkMutation(ctx, cmd, cmdArgs, &opts, cfg, stdout, stderr)
	case "move":
		return runMove(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "delete":
		return runDelete(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "text":
		return runText(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "folders":
		return runFolders(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "highlights":
		return runHighlights(ctx, cmdArgs, &opts, cfg, stdout, stderr)
	case "health":
		return runHealth(ctx, &opts, cfg, stdout, stderr)
	case "doctor":
		return runDoctor(ctx, &opts, cfgPath, cfg, stdout, stderr)
	case "verify":
		return runVerify(ctx, &opts, cfg, stdout, stderr)
	case "schema":
		return runSchema(cmdArgs, &opts, stdout, stderr)
	case "tags":
		return runTags(cmdArgs, stdout, stderr)
	default:
		if stderrJSONEnabled {
			return printUsageError(stderr, fmt.Sprintf("unknown command: %s", cmd))
		}
		fmt.Fprintf(stderr, "unknown command: %s\n\n", cmd)
		fmt.Fprintln(stderr, usageRoot())
		return 2
	}
}

func resolveConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return config.DefaultConfigPath()
}

func usageRoot() string {
	return `Usage:
  ip [global flags] <command> [args]

Global flags:
  --config <path>       Override config path
  --format table|plain|json|ndjson   Output format (default from config or ndjson)
  --json                Output JSON (alias for --format json)
  --plain               Output plain text (alias for --format plain)
  --ndjson              Output NDJSON (alias for --format ndjson)
  --jsonl               Output NDJSON (alias for --format ndjson)
  --output <file>       Write output to file ('-' for stdout)
  --stderr-json         Emit errors as JSON on stderr
  --timeout 15s         HTTP timeout
  --retry N             Retry count for transient errors
  --retry-backoff 500ms Retry backoff base duration
  --api-base <url>      API base URL (default https://www.instapaper.com)
  --debug               Debug output
  --debug-json          Debug output as JSON lines
  --quiet               Less output
  --verbose             More output
  --dry-run             Preview actions without making changes
  --idempotent          Ignore already-in-state errors when possible
  -h, --help            Show help
  --version             Show version

Commands:
  help [command]
  version
  config path|show|get|set|unset
  auth login|status|logout
  add <url|-> [--folder <id|"Title">] [--title ...] [--tags "a,b"]
  list [--folder unread|starred|archive|<id>|"Title"] [--limit N] [--tag name] [--have ...] [--highlights ...] [--fields ...] [--cursor <file>|--cursor-dir <dir>] [--since <bound>] [--until <bound>] [--updated-since <time>] [--max-pages N] [--select <expr>]
  export [--folder ...] [--tag ...] [--limit N] [--fields ...] [--cursor <file>|--cursor-dir <dir>] [--since <bound>] [--until <bound>] [--updated-since <time>] [--max-pages N] [--select <expr>] [--output-dir <dir>]
  import [--input-format plain|csv|ndjson] [--input <file>|-]
  help ai|agent
  progress <bookmark_id> --progress <0..1> --timestamp <unix>
  archive <bookmark_id>
  unarchive <bookmark_id>
  star <bookmark_id>
  unstar <bookmark_id>
  move <bookmark_id> --folder <folder_id|"Title">
  delete <bookmark_id> --yes-really-delete
  text <bookmark_id> [--out <file>] [--open]
  folders list|add|delete|order
  highlights list|add|delete
  health
  doctor
  verify
  schema [bookmarks|folders|highlights|auth|config]
  tags list|rename|delete
`
}

func runHelp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stdout, usageRoot())
		return 0
	}
	switch args[0] {
	case "config":
		fmt.Fprintln(stdout, usageConfig())
	case "auth":
		if len(args) > 1 && args[1] == "login" {
			fmt.Fprintln(stdout, usageAuthLogin())
		} else {
			fmt.Fprintln(stdout, usageAuth())
		}
	case "add":
		fmt.Fprintln(stdout, usageAdd())
	case "list":
		fmt.Fprintln(stdout, usageList())
	case "export":
		fmt.Fprintln(stdout, usageExport())
	case "import":
		fmt.Fprintln(stdout, usageImport())
	case "archive":
		fmt.Fprintln(stdout, usageBookmarkMutation("archive"))
	case "unarchive":
		fmt.Fprintln(stdout, usageBookmarkMutation("unarchive"))
	case "star":
		fmt.Fprintln(stdout, usageBookmarkMutation("star"))
	case "unstar":
		fmt.Fprintln(stdout, usageBookmarkMutation("unstar"))
	case "move":
		fmt.Fprintln(stdout, usageMove())
	case "delete":
		fmt.Fprintln(stdout, usageDelete())
	case "progress":
		fmt.Fprintln(stdout, usageProgress())
	case "text":
		fmt.Fprintln(stdout, usageText())
	case "folders":
		fmt.Fprintln(stdout, usageFolders())
	case "highlights":
		fmt.Fprintln(stdout, usageHighlights())
	case "ai", "agent":
		fmt.Fprintln(stdout, usageAgent())
	case "health":
		fmt.Fprintln(stdout, usageHealth())
	case "doctor":
		fmt.Fprintln(stdout, usageDoctor())
	case "verify":
		fmt.Fprintln(stdout, usageVerify())
	case "schema":
		fmt.Fprintln(stdout, usageSchema())
	case "tags":
		fmt.Fprintln(stdout, usageTags())
	default:
		if stderrJSONEnabled {
			return printUsageError(stderr, fmt.Sprintf("unknown command: %s", args[0]))
		}
		fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
		fmt.Fprintln(stderr, usageRoot())
		return 2
	}
	return 0
}

// --- config ---
func runConfig(args []string, cfgPath string, opts *GlobalOptions, stdout, stderr io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageConfig())
		return 0
	}
	if len(args) == 0 {
		return printUsageError(stderr, "usage: ip config path|show|get|set|unset")
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(stdout, cfgPath)
		return 0
	case "show":
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return printError(stderr, err)
		}
		if opts != nil {
			switch {
			case strings.EqualFold(opts.Format, "json"):
				if err := output.WriteJSON(stdout, cfg); err != nil {
					return printError(stderr, err)
				}
				return 0
			case isNDJSONFormat(opts.Format):
				if err := output.WriteJSONLine(stdout, cfg); err != nil {
					return printError(stderr, err)
				}
				return 0
			case strings.EqualFold(opts.Format, "plain"):
				if err := printConfigPlain(stdout, cfg); err != nil {
					return printError(stderr, err)
				}
				return 0
			}
		}
		if err := printConfig(stdout, cfg); err != nil {
			return printError(stderr, err)
		}
		return 0
	case "get":
		if len(args) != 2 {
			return printUsageError(stderr, "usage: ip config get <key>")
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return printError(stderr, err)
		}
		val, ok, err := configGet(cfg, args[1])
		if err != nil {
			return printError(stderr, err)
		}
		if !ok {
			return printError(stderr, fmt.Errorf("unknown config key: %s", args[1]))
		}
		if strings.EqualFold(opts.Format, "json") {
			if err := output.WriteJSON(stdout, map[string]any{"key": args[1], "value": val}); err != nil {
				return printError(stderr, err)
			}
			return 0
		}
		if isNDJSONFormat(opts.Format) {
			if err := output.WriteJSONLine(stdout, map[string]any{"key": args[1], "value": val}); err != nil {
				return printError(stderr, err)
			}
			return 0
		}
		fmt.Fprintf(stdout, "%s=%v\n", args[1], val)
		return 0
	case "set":
		if len(args) != 3 {
			return printUsageError(stderr, "usage: ip config set <key> <value>")
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return printError(stderr, err)
		}
		if err := configSet(cfg, args[1], args[2]); err != nil {
			return printError(stderr, err)
		}
		if err := cfg.Save(cfgPath); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintf(stdout, "Set %s\n", args[1])
		}
		return 0
	case "unset":
		if len(args) != 2 {
			return printUsageError(stderr, "usage: ip config unset <key>")
		}
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return printError(stderr, err)
		}
		if err := configUnset(cfg, args[1]); err != nil {
			return printError(stderr, err)
		}
		if err := cfg.Save(cfgPath); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintf(stdout, "Unset %s\n", args[1])
		}
		return 0
	default:
		return printUsageError(stderr, "usage: ip config path|show|get|set|unset")
	}
}

// --- helpers ---
func consumerCredsFromEnvOrConfig(cfg *config.Config) (string, string) {
	ck := os.Getenv("INSTAPAPER_CONSUMER_KEY")
	cs := os.Getenv("INSTAPAPER_CONSUMER_SECRET")
	if ck == "" {
		ck = cfg.ConsumerKey
	}
	if cs == "" {
		cs = cfg.ConsumerSecret
	}
	return ck, cs
}

func requireClient(opts *GlobalOptions, cfg *config.Config, requireAuth bool, stderr io.Writer) (*instapaper.Client, string, string, error) {
	ck, cs := consumerCredsFromEnvOrConfig(cfg)
	if ck == "" || cs == "" {
		return nil, "", "", errors.New("missing consumer key/secret: set INSTAPAPER_CONSUMER_KEY and INSTAPAPER_CONSUMER_SECRET")
	}
	var tok *oauth1.Token
	if cfg.HasAuth() {
		tok = &oauth1.Token{Key: cfg.OAuthToken, Secret: cfg.OAuthTokenSecret}
	}
	if requireAuth && tok == nil {
		return nil, "", "", errors.New("not logged in; run: ip auth login")
	}
	client, err := instapaper.NewClient(opts.APIBase, ck, cs, tok, opts.Timeout)
	if err != nil {
		return nil, "", "", err
	}
	if opts.DebugJSON {
		client.EnableDebugJSON(stderr)
	} else if opts.Debug {
		client.EnableDebug(stderr)
	}
	if opts.RetryCount > 0 {
		client.SetRetry(opts.RetryCount, opts.RetryBackoff)
	}
	return client, ck, cs, nil
}

func parseInt64(arg string) (int64, error) {
	v, err := strconv.ParseInt(arg, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", arg)
	}
	return v, nil
}

func reorderFlags(args []string) []string {
	flags := []string{}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		positionals = append(positionals, arg)
	}
	return append(flags, positionals...)
}

func resolveListFolderID(ctx context.Context, client *instapaper.Client, folder string) (string, error) {
	if folder == "" {
		return "unread", nil
	}
	lower := strings.ToLower(folder)
	if lower == "unread" || lower == "starred" || lower == "archive" {
		return lower, nil
	}
	if _, err := strconv.ParseInt(folder, 10, 64); err == nil {
		return folder, nil
	}
	folders, err := client.ListFolders(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range folders {
		if strings.EqualFold(f.Title, folder) {
			return strconv.FormatInt(int64(f.FolderID), 10), nil
		}
	}
	return "", fmt.Errorf("folder not found: %s", folder)
}

func resolveUserFolderID(ctx context.Context, client *instapaper.Client, folder string) (string, error) {
	if folder == "" || strings.EqualFold(folder, "unread") {
		return "", nil // omit folder_id
	}
	if strings.EqualFold(folder, "archive") {
		return "", fmt.Errorf("'archive' is not a user folder; use --archive instead")
	}
	if strings.EqualFold(folder, "starred") {
		return "", fmt.Errorf("'starred' is not a user folder; star after adding instead")
	}
	if _, err := strconv.ParseInt(folder, 10, 64); err == nil {
		return folder, nil
	}
	folders, err := client.ListFolders(ctx)
	if err != nil {
		return "", err
	}
	for _, f := range folders {
		if strings.EqualFold(f.Title, folder) {
			return strconv.FormatInt(int64(f.FolderID), 10), nil
		}
	}
	return "", fmt.Errorf("folder not found: %s", folder)
}

// --- auth ---
func runAuth(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, cfgPath string, stdout, stderr io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageAuth())
		return 0
	}
	if len(args) == 0 {
		return printUsageError(stderr, "usage: ip auth login|status|logout")
	}
	switch args[0] {
	case "status":
		if strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format) {
			payload := map[string]any{
				"logged_in": cfg.HasAuth(),
			}
			if cfg.HasAuth() {
				payload["user"] = map[string]any{
					"user_id":  cfg.User.UserID,
					"username": cfg.User.Username,
				}
			}
			if isNDJSONFormat(opts.Format) {
				if err := output.WriteJSONLine(stdout, payload); err != nil {
					return printError(stderr, err)
				}
				return 0
			}
			if err := output.WriteJSON(stdout, payload); err != nil {
				return printError(stderr, err)
			}
			return 0
		}
		if cfg.HasAuth() {
			fmt.Fprintf(stdout, "Logged in as %s (user_id=%d)\n", cfg.User.Username, cfg.User.UserID)
			return 0
		}
		fmt.Fprintln(stdout, "Not logged in")
		return 0
	case "logout":
		cfg.ClearAuth()
		if err := cfg.Save(cfgPath); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintln(stdout, "Logged out")
		}
		return 0
	case "login":
		return runAuthLogin(ctx, args[1:], opts, cfg, cfgPath, stdout, stderr)
	default:
		return printUsageError(stderr, "usage: ip auth login|status|logout")
	}
}

func runAuthLogin(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, cfgPath string, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var noInput bool
	var username string
	var passwordStdin bool
	var consumerKey string
	var consumerSecret string
	var saveConsumer bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.BoolVar(&noInput, "no-input", false, "Disable prompts; fail if required values are missing")
	fs.StringVar(&username, "username", "", "Email or username")
	fs.BoolVar(&passwordStdin, "password-stdin", false, "Read password from stdin")
	fs.StringVar(&consumerKey, "consumer-key", "", "Instapaper API consumer key")
	fs.StringVar(&consumerSecret, "consumer-secret", "", "Instapaper API consumer secret")
	fs.BoolVar(&saveConsumer, "save-consumer", false, "Save consumer key/secret in config")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageAuthLogin(), fs)
		return 0
	}

	if consumerKey == "" {
		consumerKey = os.Getenv("INSTAPAPER_CONSUMER_KEY")
		if consumerKey == "" {
			consumerKey = cfg.ConsumerKey
		}
	}
	if consumerSecret == "" {
		consumerSecret = os.Getenv("INSTAPAPER_CONSUMER_SECRET")
		if consumerSecret == "" {
			consumerSecret = cfg.ConsumerSecret
		}
	}
	if consumerKey == "" || consumerSecret == "" {
		return printError(stderr, errors.New("missing consumer key/secret (set env INSTAPAPER_CONSUMER_KEY/INSTAPAPER_CONSUMER_SECRET or pass flags)"))
	}

	interactive := isTTY(os.Stdin)
	if username == "" {
		if noInput || !interactive {
			return printUsageError(stderr, "missing --username (stdin is not a TTY)")
		}
		u, err := prompt.ReadLine(os.Stdin, stderr, "Email or username: ")
		if err != nil {
			return printError(stderr, err)
		}
		username = strings.TrimSpace(u)
	}

	var password string
	if passwordStdin {
		if isTTY(os.Stdin) {
			return printUsageError(stderr, "--password-stdin requires piped input (stdin is a TTY)")
		}
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return printError(stderr, err)
		}
		password = strings.TrimSpace(string(b))
	} else {
		if noInput || !interactive {
			return printUsageError(stderr, "missing password; use --password-stdin or run interactively")
		}
		pw, err := prompt.ReadPassword(stderr, "Password, if you have one: ", os.Stdin)
		if err != nil {
			return printError(stderr, err)
		}
		password = pw
	}

	client, err := instapaper.NewClient(opts.APIBase, consumerKey, consumerSecret, nil, opts.Timeout)
	if err != nil {
		return printError(stderr, err)
	}
	if opts.DebugJSON {
		client.EnableDebugJSON(stderr)
	} else if opts.Debug {
		client.EnableDebug(stderr)
	}
	if opts.RetryCount > 0 {
		client.SetRetry(opts.RetryCount, opts.RetryBackoff)
	}
	ok, sk, err := client.XAuthAccessToken(ctx, username, password)
	if err != nil {
		return printError(stderr, err)
	}

	cfg.OAuthToken = ok
	cfg.OAuthTokenSecret = sk
	cfg.APIBase = opts.APIBase
	if saveConsumer {
		cfg.ConsumerKey = consumerKey
		cfg.ConsumerSecret = consumerSecret
	}

	client2, err := instapaper.NewClient(opts.APIBase, consumerKey, consumerSecret, &oauth1.Token{Key: ok, Secret: sk}, opts.Timeout)
	if err != nil {
		return printError(stderr, err)
	}
	if opts.DebugJSON {
		client2.EnableDebugJSON(stderr)
	} else if opts.Debug {
		client2.EnableDebug(stderr)
	}
	if opts.RetryCount > 0 {
		client2.SetRetry(opts.RetryCount, opts.RetryBackoff)
	}
	u, err := client2.VerifyCredentials(ctx)
	if err != nil {
		return printError(stderr, err)
	}
	cfg.User.UserID = int64(u.UserID)
	cfg.User.Username = u.Username

	if err := cfg.Save(cfgPath); err != nil {
		return printError(stderr, err)
	}
	if !opts.Quiet {
		fmt.Fprintf(stdout, "Logged in as %s (user_id=%d)\n", cfg.User.Username, cfg.User.UserID)
	}
	return 0
}

// --- bookmarks ---
func runAdd(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var title string
	var desc string
	var folder string
	var archive bool
	var tags string
	var resolveFinal bool
	var resolveFinalSet bool
	var contentFile string
	var privateSource string
	var batch int
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&title, "title", "", "Title")
	fs.StringVar(&desc, "description", "", "Description")
	fs.StringVar(&folder, "folder", "", "User folder: <id>|\"Title\" (omit for Unread)")
	fs.BoolVar(&archive, "archive", false, "Archive immediately")
	fs.StringVar(&tags, "tags", "", "Comma-separated tags")
	fs.Func("resolve-final-url", "Resolve redirects (1/0)", func(v string) error {
		resolveFinalSet = true
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y":
			resolveFinal = true
		case "0", "false", "no", "n":
			resolveFinal = false
		default:
			return fmt.Errorf("invalid value: %s", v)
		}
		return nil
	})
	fs.StringVar(&contentFile, "content-file", "", "Path to HTML content to send as 'content'")
	fs.StringVar(&privateSource, "private-source", "", "Set is_private_from_source (requires content)")
	fs.IntVar(&batch, "batch", 0, "Process items in batches of N (0 = all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageAdd(), fs)
		return 0
	}
	if batch < 0 {
		return printUsageError(stderr, "--batch must be >= 0")
	}
	remaining := fs.Args()
	if len(remaining) != 1 {
		return printUsageError(stderr, "usage: ip add <url|-> [flags]")
	}
	urlArg := remaining[0]

	var content string
	if contentFile != "" {
		b, err := os.ReadFile(contentFile)
		if err != nil {
			return printError(stderr, err)
		}
		content = string(b)
	}

	resolveFinalURL := cfg.Defaults.ResolveFinalURLValue()
	if resolveFinalSet {
		resolveFinalURL = resolveFinal
	}
	if opts.DryRun {
		records := []map[string]any{}
		addRecord := func(u string) {
			records = append(records, map[string]any{
				"url":               u,
				"title":             title,
				"description":       desc,
				"folder":            folder,
				"archive":           archive,
				"tags":              strings.Split(strings.TrimSpace(tags), ","),
				"resolve_final_url": resolveFinalURL,
				"content_file":      contentFile,
				"private_source":    privateSource,
			})
		}
		if urlArg == "-" {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				u := strings.TrimSpace(scanner.Text())
				if u == "" {
					continue
				}
				addRecord(u)
			}
			if err := scanner.Err(); err != nil {
				return printError(stderr, err)
			}
		} else {
			addRecord(urlArg)
		}
		return emitDryRunRecords(stdout, opts.Format, "add", records)
	}

	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}

	folderID, err := resolveUserFolderID(ctx, client, folder)
	if err != nil {
		return printError(stderr, err)
	}

	makeReq := func(u string) instapaper.AddBookmarkRequest {
		var tagsList []string
		if tags != "" {
			parts := strings.Split(tags, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					tagsList = append(tagsList, p)
				}
			}
		}
		return instapaper.AddBookmarkRequest{
			URL:             u,
			Title:           title,
			Description:     desc,
			FolderID:        folderID,
			ResolveFinalURL: resolveFinalURL,
			Archived:        archive,
			Tags:            tagsList,
			Content:         content,
			PrivateSource:   privateSource,
		}
	}

	addOne := func(u string) error {
		bm, err := client.AddBookmark(ctx, makeReq(u))
		if err != nil {
			return err
		}
		if opts.Quiet {
			fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
			return nil
		}
		t := bm.Title
		if t == "" {
			t = u
		}
		fmt.Fprintf(stdout, "Added %d: %s\n", int64(bm.BookmarkID), t)
		return nil
	}

	if urlArg == "-" {
		scanner := bufio.NewScanner(os.Stdin)
		exit := 0
		count := 0
		for scanner.Scan() {
			u := strings.TrimSpace(scanner.Text())
			if u == "" {
				continue
			}
			if err := addOne(u); err != nil {
				code := exitCodeForError(err)
				if code > exit {
					exit = code
				}
				writeErrorLine(stderr, fmt.Errorf("adding %s: %v", u, err))
			}
			count++
			if batch > 0 && count%batch == 0 && opts.RetryBackoff > 0 {
				time.Sleep(opts.RetryBackoff)
			}
		}
		if err := scanner.Err(); err != nil {
			return printError(stderr, err)
		}
		return exit
	}

	if err := addOne(urlArg); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runList(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var folder string
	var limit int
	var tag string
	var have string
	var highlights string
	var fields string
	var cursorPath string
	var cursorDir string
	var since string
	var until string
	var updatedSince string
	var maxPages int
	var selectExpr string
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&folder, "folder", "unread", "Folder: unread|starred|archive|<id>|\"Title\"")
	fs.IntVar(&limit, "limit", cfg.Defaults.ListLimit, "Limit (0 = no limit, max 500)")
	fs.StringVar(&tag, "tag", "", "Tag name (when provided, folder is ignored)")
	fs.StringVar(&have, "have", "", "Comma-separated IDs to exclude (id:progress:timestamp)")
	fs.StringVar(&highlights, "highlights", "", "Comma-separated bookmark IDs for highlights")
	fs.StringVar(&fields, "fields", "", "Comma-separated fields (json/ndjson only)")
	fs.StringVar(&cursorPath, "cursor", "", "Path to cursor file for incremental sync")
	fs.StringVar(&cursorDir, "cursor-dir", "", "Directory for auto cursor files")
	fs.StringVar(&since, "since", "", "Filter bookmarks since a bound (bookmark_id:<id> or time:<rfc3339|unix>)")
	fs.StringVar(&until, "until", "", "Filter bookmarks up to a bound (bookmark_id:<id> or time:<rfc3339|unix>)")
	fs.StringVar(&updatedSince, "updated-since", "", "Filter by updated time (progress_timestamp or time)")
	fs.IntVar(&maxPages, "max-pages", 200, "Max pages when --limit is 0")
	fs.StringVar(&selectExpr, "select", "", "Filter results client-side (e.g. starred=1,tag~news)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageList(), fs)
		return 0
	}

	if limit < 0 || limit > 500 {
		return printUsageError(stderr, fmt.Sprintf("invalid --limit %d (expected 0..500)", limit))
	}
	if maxPages < 0 {
		return printUsageError(stderr, "--max-pages must be >= 0")
	}
	if fields != "" && !strings.EqualFold(opts.Format, "json") && !isNDJSONFormat(opts.Format) {
		return printUsageError(stderr, "--fields requires --json or --ndjson output")
	}
	if since != "" && updatedSince != "" {
		return printUsageError(stderr, "use only one of --since or --updated-since")
	}

	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}

	folderID := ""
	if tag == "" {
		folderID, err = resolveListFolderID(ctx, client, folder)
		if err != nil {
			return printError(stderr, err)
		}
	}
	if cursorPath == "" && cursorDir != "" {
		cursorPath = resolveCursorPath(cursorDir, folderID, tag)
	}

	sinceBound, err := parseBoundSpec(since, "bookmark_id")
	if err != nil {
		return printUsageError(stderr, err.Error())
	}
	if updatedSince != "" {
		updatedBound, err := parseUpdatedBound(updatedSince)
		if err != nil {
			return printUsageError(stderr, err.Error())
		}
		sinceBound = updatedBound
	}
	untilBound, err := parseBoundSpec(until, "bookmark_id")
	if err != nil {
		return printUsageError(stderr, err.Error())
	}

	resp, err := listBookmarks(ctx, client, listBookmarksParams{
		Limit:      limit,
		FolderID:   folderID,
		Tag:        tag,
		Have:       have,
		Highlights: highlights,
		Fields:     fields,
		CursorPath: cursorPath,
		MaxPages:   maxPages,
	})
	if err != nil {
		return printError(stderr, err)
	}
	resp.Bookmarks = filterBookmarksByBounds(resp.Bookmarks, sinceBound, untilBound)
	if selectExpr != "" {
		filtered, err := filterBookmarksBySelect(resp.Bookmarks, selectExpr)
		if err != nil {
			return printUsageError(stderr, err.Error())
		}
		resp.Bookmarks = filtered
	}
	verbosef(opts, stderr, "list: bookmarks=%d", len(resp.Bookmarks))
	if fields != "" && (strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format)) {
		if err := output.PrintBookmarksWithFields(stdout, opts.Format, resp.Bookmarks, fields); err != nil {
			return printError(stderr, err)
		}
		return 0
	}
	if err := output.PrintBookmarks(stdout, opts.Format, resp.Bookmarks); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runExport(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var folder string
	var limit int
	var tag string
	var have string
	var fields string
	var cursorPath string
	var cursorDir string
	var since string
	var until string
	var updatedSince string
	var maxPages int
	var selectExpr string
	var outputDir string
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&folder, "folder", "unread", "Folder: unread|starred|archive|<id>|\"Title\"")
	fs.IntVar(&limit, "limit", 0, "Limit (0 = no limit, max 500)")
	fs.StringVar(&tag, "tag", "", "Tag name (when provided, folder is ignored)")
	fs.StringVar(&have, "have", "", "Comma-separated IDs to exclude (id:progress:timestamp)")
	fs.StringVar(&fields, "fields", "", "Comma-separated fields (json/ndjson only)")
	fs.StringVar(&cursorPath, "cursor", "", "Path to cursor file for incremental sync")
	fs.StringVar(&cursorDir, "cursor-dir", "", "Directory for auto cursor files")
	fs.StringVar(&since, "since", "", "Filter bookmarks since a bound (bookmark_id:<id> or time:<rfc3339|unix>)")
	fs.StringVar(&until, "until", "", "Filter bookmarks up to a bound (bookmark_id:<id> or time:<rfc3339|unix>)")
	fs.StringVar(&updatedSince, "updated-since", "", "Filter by updated time (progress_timestamp or time)")
	fs.IntVar(&maxPages, "max-pages", 200, "Max pages when --limit is 0")
	fs.StringVar(&selectExpr, "select", "", "Filter results client-side (e.g. starred=1,tag~news)")
	fs.StringVar(&outputDir, "output-dir", "", "Write each page as NDJSON into this directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageExport(), fs)
		return 0
	}
	if limit < 0 || limit > 500 {
		return printUsageError(stderr, fmt.Sprintf("invalid --limit %d (expected 0..500)", limit))
	}
	if maxPages < 0 {
		return printUsageError(stderr, "--max-pages must be >= 0")
	}
	if fields != "" && !strings.EqualFold(opts.Format, "json") && !isNDJSONFormat(opts.Format) {
		return printUsageError(stderr, "--fields requires --json or --ndjson output")
	}
	if since != "" && updatedSince != "" {
		return printUsageError(stderr, "use only one of --since or --updated-since")
	}
	if outputDir != "" && opts.OutputPath != "" {
		return printUsageError(stderr, "--output and --output-dir cannot be used together")
	}
	if outputDir != "" && !isNDJSONFormat(opts.Format) {
		return printUsageError(stderr, "--output-dir requires --format ndjson")
	}

	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}

	folderID := ""
	if tag == "" {
		folderID, err = resolveListFolderID(ctx, client, folder)
		if err != nil {
			return printError(stderr, err)
		}
	}
	if cursorPath == "" && cursorDir != "" {
		cursorPath = resolveCursorPath(cursorDir, folderID, tag)
	}

	sinceBound, err := parseBoundSpec(since, "bookmark_id")
	if err != nil {
		return printUsageError(stderr, err.Error())
	}
	if updatedSince != "" {
		updatedBound, err := parseUpdatedBound(updatedSince)
		if err != nil {
			return printUsageError(stderr, err.Error())
		}
		sinceBound = updatedBound
	}
	untilBound, err := parseBoundSpec(until, "bookmark_id")
	if err != nil {
		return printUsageError(stderr, err.Error())
	}

	var pageWriter *pagedExportWriter
	if outputDir != "" {
		pageWriter, err = newPagedExportWriter(outputDir, folderID, tag, fields)
		if err != nil {
			return printError(stderr, err)
		}
	}
	var selectFilters []selectFilter
	if selectExpr != "" {
		selectFilters, err = parseSelectExpr(selectExpr)
		if err != nil {
			return printUsageError(stderr, err.Error())
		}
	}

	resp, err := listBookmarks(ctx, client, listBookmarksParams{
		Limit:      limit,
		FolderID:   folderID,
		Tag:        tag,
		Have:       have,
		Fields:     fields,
		CursorPath: cursorPath,
		MaxPages:   maxPages,
		PageHandler: func(page []instapaper.Bookmark, pageIndex int) error {
			if pageWriter == nil {
				return nil
			}
			filtered := filterBookmarksByBounds(page, sinceBound, untilBound)
			if len(selectFilters) > 0 {
				filtered = filterBookmarksBySelectFilters(filtered, selectFilters)
			}
			if len(filtered) == 0 {
				return nil
			}
			return pageWriter.WritePage(pageIndex, filtered)
		},
		DiscardOutput: outputDir != "",
	})
	if err != nil {
		return printError(stderr, err)
	}
	if outputDir != "" {
		if pageWriter != nil && !opts.Quiet {
			fmt.Fprintf(stdout, "Wrote %d pages to %s\n", pageWriter.pages, outputDir)
		}
		return 0
	}
	resp.Bookmarks = filterBookmarksByBounds(resp.Bookmarks, sinceBound, untilBound)
	if len(selectFilters) > 0 {
		resp.Bookmarks = filterBookmarksBySelectFilters(resp.Bookmarks, selectFilters)
	}
	verbosef(opts, stderr, "export: bookmarks=%d", len(resp.Bookmarks))
	if fields != "" && (strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format)) {
		if err := output.PrintBookmarksWithFields(stdout, opts.Format, resp.Bookmarks, fields); err != nil {
			return printError(stderr, err)
		}
		return 0
	}
	if err := output.PrintBookmarks(stdout, opts.Format, resp.Bookmarks); err != nil {
		return printError(stderr, err)
	}
	return 0
}

type importItem struct {
	URL         string
	Title       string
	Description string
	Tags        []string
	Folder      string
	Archive     bool
}

func runImport(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var inputPath string
	var inputFormat string
	var folder string
	var tags string
	var archive bool
	var progressJSON bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&inputPath, "input", "-", "Input file ('-' for stdin)")
	fs.StringVar(&inputFormat, "input-format", "plain", "Input format: plain|csv|ndjson")
	fs.StringVar(&folder, "folder", "", "Default folder for imported items")
	fs.StringVar(&tags, "tags", "", "Default tags for imported items (comma-separated)")
	fs.BoolVar(&archive, "archive", false, "Archive imported items")
	fs.BoolVar(&progressJSON, "progress-json", false, "Emit progress as NDJSON on stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageImport(), fs)
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(inputFormat)) {
	case "plain", "csv", "ndjson", "jsonl":
	default:
		return printUsageError(stderr, fmt.Sprintf("invalid --input-format %q (expected plain, csv, or ndjson)", inputFormat))
	}
	items, err := readImportItems(inputPath, inputFormat, folder, tags, archive)
	if err != nil {
		return printError(stderr, err)
	}
	if len(items) == 0 {
		return 0
	}
	if opts.DryRun {
		return emitDryRunItems(stdout, opts.Format, "import", items)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	emitter := newProgressEmitter(progressJSON, stderr, "import", len(items))
	emitter.Start()
	folderCache := map[string]string{}
	exit := 0
	for _, it := range items {
		folderID := ""
		if it.Folder != "" {
			if cached, ok := folderCache[it.Folder]; ok {
				folderID = cached
			} else if _, err := strconv.ParseInt(it.Folder, 10, 64); err == nil {
				folderID = it.Folder
			} else {
				id, err := resolveUserFolderID(ctx, client, it.Folder)
				if err != nil {
					exit = exitCodeForError(err)
					writeErrorLine(stderr, err)
					continue
				}
				folderCache[it.Folder] = id
				folderID = id
			}
		}
		req := instapaper.AddBookmarkRequest{
			URL:             it.URL,
			Title:           it.Title,
			Description:     it.Description,
			FolderID:        folderID,
			ResolveFinalURL: cfg.Defaults.ResolveFinalURLValue(),
			Archived:        it.Archive,
			Tags:            it.Tags,
		}
		bm, err := client.AddBookmark(ctx, req)
		if err != nil {
			exit = exitCodeForError(err)
			emitter.ItemError(map[string]any{"url": it.URL}, err)
			writeErrorLine(stderr, fmt.Errorf("adding %s: %v", it.URL, err))
			continue
		}
		emitter.ItemSuccess(map[string]any{"bookmark_id": int64(bm.BookmarkID), "url": it.URL})
		if opts.Quiet {
			fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
			continue
		}
		if strings.EqualFold(opts.Format, "json") {
			_ = output.WriteJSONLine(stdout, bm)
			continue
		}
		if isNDJSONFormat(opts.Format) {
			_ = output.WriteJSONLine(stdout, bm)
			continue
		}
		fmt.Fprintf(stdout, "Added %d: %s\n", int64(bm.BookmarkID), bm.Title)
	}
	emitter.Done()
	return exit
}

func runHealth(ctx context.Context, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	u, err := client.VerifyCredentials(ctx)
	if err != nil {
		return printError(stderr, err)
	}
	payload := map[string]any{
		"status": "ok",
		"user": map[string]any{
			"user_id":  u.UserID,
			"username": u.Username,
		},
	}
	if strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format) {
		if err := writeJSONByFormat(stdout, opts.Format, payload); err != nil {
			return printError(stderr, err)
		}
		return 0
	}
	fmt.Fprintf(stdout, "OK %s (user_id=%d)\n", u.Username, u.UserID)
	return 0
}

func runVerify(ctx context.Context, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	ck, cs := consumerCredsFromEnvOrConfig(cfg)
	hasAuth := cfg.HasAuth()
	result := map[string]any{
		"consumer_key":    ck != "",
		"consumer_secret": cs != "",
		"auth":            hasAuth,
	}
	ok := (ck != "" && cs != "")
	if hasAuth {
		client, _, _, err := requireClient(opts, cfg, true, stderr)
		if err != nil {
			result["network"] = false
			ok = false
		} else if _, err := client.VerifyCredentials(ctx); err != nil {
			result["network"] = false
			result["error"] = err.Error()
			ok = false
		} else {
			result["network"] = true
		}
	} else {
		result["network"] = false
		ok = false
	}
	result["ok"] = ok
	if strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format) {
		if err := writeJSONByFormat(stdout, opts.Format, result); err != nil {
			return printError(stderr, err)
		}
		return 0
	}
	fmt.Fprintf(stdout, "consumer_key=%t\nconsumer_secret=%t\nauth=%t\nnetwork=%t\nok=%t\n",
		result["consumer_key"], result["consumer_secret"], result["auth"], result["network"], result["ok"])
	return 0
}

type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

func runDoctor(ctx context.Context, opts *GlobalOptions, cfgPath string, cfg *config.Config, stdout, stderr io.Writer) int {
	configExists := false
	if cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			configExists = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return printError(stderr, err)
		}
	}

	ck := os.Getenv("INSTAPAPER_CONSUMER_KEY")
	ckSource := "env"
	if ck == "" {
		ck = cfg.ConsumerKey
		if ck != "" {
			ckSource = "config"
		} else {
			ckSource = "missing"
		}
	}
	cs := os.Getenv("INSTAPAPER_CONSUMER_SECRET")
	csSource := "env"
	if cs == "" {
		cs = cfg.ConsumerSecret
		if cs != "" {
			csSource = "config"
		} else {
			csSource = "missing"
		}
	}
	ckOK := ckSource != "missing"
	csOK := csSource != "missing"
	authOK := cfg.HasAuth()

	checks := []doctorCheck{
		{
			Name:   "config_file",
			OK:     true,
			Detail: fmt.Sprintf("%s (exists=%t)", cfgPath, configExists),
		},
		{
			Name:   "consumer_key",
			OK:     ckOK,
			Detail: ckSource,
		},
		{
			Name:   "consumer_secret",
			OK:     csOK,
			Detail: csSource,
		},
		{
			Name:   "auth",
			OK:     authOK,
			Detail: "config",
		},
	}
	if !ckOK {
		checks[1].Hint = "set INSTAPAPER_CONSUMER_KEY or ip config set consumer_key <value>"
	}
	if !csOK {
		checks[2].Hint = "set INSTAPAPER_CONSUMER_SECRET or ip config set consumer_secret <value>"
	}
	if !authOK {
		checks[3].Hint = "run ip auth login to store tokens"
	}

	networkOK := false
	networkDetail := "skipped (missing credentials or auth)"
	if ckOK && csOK && authOK {
		client, _, _, err := requireClient(opts, cfg, true, stderr)
		if err != nil {
			networkDetail = err.Error()
		} else if user, err := client.VerifyCredentials(ctx); err != nil {
			networkDetail = err.Error()
		} else {
			networkOK = true
			networkDetail = fmt.Sprintf("user=%s (id=%d)", user.Username, user.UserID)
		}
	}
	networkCheck := doctorCheck{
		Name:   "network",
		OK:     networkOK,
		Detail: networkDetail,
	}
	if !networkOK {
		networkCheck.Hint = "ensure auth is valid and api_base is reachable"
	}
	checks = append(checks, networkCheck)

	ok := ckOK && csOK && authOK && networkOK
	result := map[string]any{
		"ok":            ok,
		"config_path":   cfgPath,
		"api_base":      opts.APIBase,
		"timeout":       opts.Timeout.String(),
		"retry":         opts.RetryCount,
		"retry_backoff": opts.RetryBackoff.String(),
		"checks":        checks,
	}

	if strings.EqualFold(opts.Format, "json") || isNDJSONFormat(opts.Format) {
		if err := writeJSONByFormat(stdout, opts.Format, result); err != nil {
			return printError(stderr, err)
		}
		return 0
	}

	fmt.Fprintf(stdout, "ok=%t\nconfig_path=%s\napi_base=%s\ntimeout=%s\nretry=%d\nretry_backoff=%s\n",
		ok, cfgPath, opts.APIBase, opts.Timeout.String(), opts.RetryCount, opts.RetryBackoff.String())
	for _, check := range checks {
		status := "ok"
		if !check.OK {
			status = "fail"
		}
		fmt.Fprintf(stdout, "%s=%s (%s)\n", check.Name, status, check.Detail)
	}
	return 0
}

func runSchema(args []string, opts *GlobalOptions, stdout, stderr io.Writer) int {
	target := "bookmarks"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		target = strings.ToLower(strings.TrimSpace(args[0]))
	}
	schema, ok := schemaForTarget(target)
	if !ok {
		return printUsageError(stderr, "usage: ip schema [bookmarks|folders|highlights|auth|config]")
	}
	if err := writeJSONByFormat(stdout, opts.Format, schema); err != nil {
		return printError(stderr, err)
	}
	return 0
}

func runTags(args []string, stdout, stderr io.Writer) int {
	msg := "tags management is not available via the Instapaper API; use `ip list --tag <name>` instead"
	return printError(stderr, errors.New(msg))
}

type listBookmarksParams struct {
	Limit         int
	FolderID      string
	Tag           string
	Have          string
	Highlights    string
	Fields        string
	CursorPath    string
	MaxPages      int
	PageHandler   func([]instapaper.Bookmark, int) error
	DiscardOutput bool
}

type cursorEntry struct {
	Hash              string  `json:"hash,omitempty"`
	Progress          float64 `json:"progress,omitempty"`
	ProgressTimestamp int64   `json:"progress_timestamp,omitempty"`
}

type listCursor struct {
	Have map[string]cursorEntry `json:"have"`
}

func listBookmarks(ctx context.Context, client *instapaper.Client, params listBookmarksParams) (instapaper.BookmarksListResponse, error) {
	var cursor *listCursor
	if params.CursorPath != "" {
		c, err := loadCursor(params.CursorPath)
		if err != nil {
			return instapaper.BookmarksListResponse{}, err
		}
		cursor = c
	}

	have := strings.TrimSpace(params.Have)
	if have != "" {
		if cursor == nil {
			cursor = &listCursor{Have: map[string]cursorEntry{}}
		}
		mergeHaveString(cursor, have)
		have = haveStringFromCursor(cursor)
	} else if cursor != nil {
		have = haveStringFromCursor(cursor)
	}

	limit := params.Limit
	if limit == 0 {
		if cursor == nil {
			cursor = &listCursor{Have: map[string]cursorEntry{}}
		}
		limit = 500
	}
	resp := instapaper.BookmarksListResponse{}
	pages := 0
	maxPages := params.MaxPages
	if maxPages <= 0 {
		maxPages = 200
	}
	for {
		pages++
		if params.Limit == 0 && pages > maxPages {
			return resp, fmt.Errorf("list exceeded max pages; use --max-pages or --limit to cap results")
		}
		r, err := client.ListBookmarks(ctx, instapaper.ListBookmarksOptions{
			Limit:      limit,
			FolderID:   params.FolderID,
			Tag:        params.Tag,
			Have:       have,
			Highlights: params.Highlights,
		})
		if err != nil {
			return resp, err
		}
		resp.User = r.User
		if !params.DiscardOutput {
			resp.Bookmarks = append(resp.Bookmarks, r.Bookmarks...)
		}
		resp.Highlights = append(resp.Highlights, r.Highlights...)
		resp.DeleteIDs = append(resp.DeleteIDs, r.DeleteIDs...)
		if params.PageHandler != nil {
			if err := params.PageHandler(r.Bookmarks, pages); err != nil {
				return resp, err
			}
		}
		if cursor != nil {
			updateCursor(cursor, r.Bookmarks, r.DeleteIDs)
			have = haveStringFromCursor(cursor)
		}
		if params.Limit > 0 || len(r.Bookmarks) == 0 {
			break
		}
	}
	if cursor != nil {
		if err := saveCursor(params.CursorPath, cursor); err != nil {
			return resp, err
		}
	}
	return resp, nil
}

func runProgress(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("progress", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var progress float64
	var timestamp int64
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.Float64Var(&progress, "progress", -1, "Read progress (0..1)")
	fs.Int64Var(&timestamp, "timestamp", 0, "Progress timestamp (unix seconds)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageProgress(), fs)
		return 0
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return printUsageError(stderr, "usage: ip progress <bookmark_id> --progress <0..1> --timestamp <unix>")
	}
	if progress < 0 || progress > 1 {
		return printUsageError(stderr, "--progress must be between 0 and 1")
	}
	if timestamp <= 0 {
		return printUsageError(stderr, "--timestamp is required (unix seconds)")
	}
	id, err := parseInt64(rest[0])
	if err != nil {
		return printError(stderr, err)
	}
	if opts.DryRun {
		_ = emitDryRunAction(stdout, opts.Format, "progress", map[string]any{
			"bookmark_id": id,
			"progress":    progress,
			"timestamp":   timestamp,
		})
		return 0
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	bm, err := client.UpdateReadProgress(ctx, id, progress, timestamp)
	if err != nil {
		return printError(stderr, err)
	}
	if opts.Quiet {
		fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
		return 0
	}
	fmt.Fprintf(stdout, "Updated progress for %d\n", int64(bm.BookmarkID))
	return 0
}

func runBookmarkMutation(ctx context.Context, cmd string, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var idsCSV string
	var stdin bool
	var batch int
	var progressJSON bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&idsCSV, "ids", "", "Comma-separated bookmark IDs")
	fs.BoolVar(&stdin, "stdin", false, "Read bookmark IDs from stdin")
	fs.IntVar(&batch, "batch", 0, "Process items in batches of N (0 = all)")
	fs.BoolVar(&progressJSON, "progress-json", false, "Emit progress as NDJSON on stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageBookmarkMutation(cmd), fs)
		return 0
	}
	ids, err := collectIDs(fs.Args(), idsCSV, stdin)
	if err != nil {
		return printUsageError(stderr, err.Error())
	}
	if len(ids) == 0 {
		return printUsageError(stderr, fmt.Sprintf("usage: ip %s <bookmark_id> [--ids ...] [--stdin]", cmd))
	}
	if batch < 0 {
		return printUsageError(stderr, "--batch must be >= 0")
	}
	if opts.DryRun {
		return emitDryRunIDs(stdout, opts.Format, cmd, ids)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}

	emitter := newProgressEmitter(progressJSON, stderr, cmd, len(ids))
	emitter.Start()
	exit := 0
	for i, id := range ids {
		var bm instapaper.Bookmark
		switch cmd {
		case "archive":
			bm, err = client.Archive(ctx, id)
		case "unarchive":
			bm, err = client.Unarchive(ctx, id)
		case "star":
			bm, err = client.Star(ctx, id)
		case "unstar":
			bm, err = client.Unstar(ctx, id)
		default:
			err = fmt.Errorf("unknown mutation: %s", cmd)
		}
		if err != nil {
			if opts.Idempotent && isAlreadyStateError(err) {
				emitter.ItemSuccess(map[string]any{"bookmark_id": id, "idempotent": true})
				if opts.Quiet {
					fmt.Fprintf(stdout, "%d\n", id)
				} else {
					fmt.Fprintf(stdout, "OK %s %d\n", cmd, id)
				}
				continue
			}
			code := exitCodeForError(err)
			if code > exit {
				exit = code
			}
			emitter.ItemError(map[string]any{"bookmark_id": id}, err)
			writeErrorLine(stderr, fmt.Errorf("%s %d: %v", cmd, id, err))
			continue
		}
		emitter.ItemSuccess(map[string]any{"bookmark_id": int64(bm.BookmarkID)})
		if opts.Quiet {
			fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
		} else {
			fmt.Fprintf(stdout, "OK %s %d\n", cmd, int64(bm.BookmarkID))
		}
		if batch > 0 && (i+1)%batch == 0 && i+1 < len(ids) && opts.RetryBackoff > 0 {
			time.Sleep(opts.RetryBackoff)
		}
	}
	emitter.Done()
	return exit
}

func runMove(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("move", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var folder string
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&folder, "folder", "", "Destination user folder: <id>|\"Title\"")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageMove(), fs)
		return 0
	}
	remaining := fs.Args()
	if len(remaining) != 1 || folder == "" {
		return printUsageError(stderr, "usage: ip move <bookmark_id> --folder <folder_id|\"Title\">")
	}
	id, err := parseInt64(remaining[0])
	if err != nil {
		return printError(stderr, err)
	}
	if opts.DryRun {
		_ = emitDryRunAction(stdout, opts.Format, "move", map[string]any{
			"bookmark_id": id,
			"folder":      folder,
		})
		return 0
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	folderID, err := resolveUserFolderID(ctx, client, folder)
	if err != nil {
		return printError(stderr, err)
	}
	if folderID == "" {
		return printUsageError(stderr, "--folder must be a user folder")
	}
	bm, err := client.Move(ctx, id, folderID)
	if err != nil {
		return printError(stderr, err)
	}
	if opts.Quiet {
		fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
		return 0
	}
	fmt.Fprintf(stdout, "Moved %d to folder %s\n", int64(bm.BookmarkID), folderID)
	return 0
}

func runDelete(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var yes bool
	var confirm string
	var idsCSV string
	var stdin bool
	var batch int
	var progressJSON bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.BoolVar(&yes, "yes-really-delete", false, "Confirm permanent deletion")
	fs.StringVar(&confirm, "confirm", "", "Confirm delete by repeating the bookmark id")
	fs.StringVar(&idsCSV, "ids", "", "Comma-separated bookmark IDs")
	fs.BoolVar(&stdin, "stdin", false, "Read bookmark IDs from stdin")
	fs.IntVar(&batch, "batch", 0, "Process items in batches of N (0 = all)")
	fs.BoolVar(&progressJSON, "progress-json", false, "Emit progress as NDJSON on stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageDelete(), fs)
		return 0
	}
	ids, err := collectIDs(fs.Args(), idsCSV, stdin)
	if err != nil {
		return printUsageError(stderr, err.Error())
	}
	if len(ids) == 0 {
		return printUsageError(stderr, "usage: ip delete <bookmark_id> --yes-really-delete|--confirm <bookmark_id>")
	}
	if batch < 0 {
		return printUsageError(stderr, "--batch must be >= 0")
	}
	if len(ids) > 1 && confirm != "" {
		return printUsageError(stderr, "--confirm is only supported for a single bookmark id")
	}
	if !opts.DryRun && !yes && confirm == "" {
		return printUsageError(stderr, "refusing: permanent delete requires --yes-really-delete or --confirm <bookmark_id>")
	}
	if confirm != "" && fmt.Sprintf("%d", ids[0]) != confirm {
		return printError(stderr, fmt.Errorf("--confirm must match bookmark id"))
	}
	if opts.DryRun {
		return emitDryRunIDs(stdout, opts.Format, "delete", ids)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	emitter := newProgressEmitter(progressJSON, stderr, "delete", len(ids))
	emitter.Start()
	exit := 0
	for i, id := range ids {
		if err := client.DeleteBookmark(ctx, id); err != nil {
			code := exitCodeForError(err)
			if code > exit {
				exit = code
			}
			emitter.ItemError(map[string]any{"bookmark_id": id}, err)
			writeErrorLine(stderr, fmt.Errorf("delete %d: %v", id, err))
		} else {
			emitter.ItemSuccess(map[string]any{"bookmark_id": id})
			if !opts.Quiet {
				fmt.Fprintf(stdout, "Deleted %d\n", id)
			}
		}
		if batch > 0 && (i+1)%batch == 0 && i+1 < len(ids) && opts.RetryBackoff > 0 {
			time.Sleep(opts.RetryBackoff)
		}
	}
	emitter.Done()
	return exit
}

func runText(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	args = reorderFlags(args)
	fs := flag.NewFlagSet("text", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var outPath string
	var openIt bool
	var stdin bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&outPath, "out", "", "Write HTML to file")
	fs.BoolVar(&openIt, "open", false, "Open the output file in default browser")
	fs.BoolVar(&stdin, "stdin", false, "Read bookmark IDs from stdin")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageText(), fs)
		return 0
	}
	remaining := fs.Args()
	if stdin && openIt {
		return printUsageError(stderr, "--open is not supported with --stdin")
	}
	var ids []int64
	var err error
	if stdin {
		ids, err = collectIDs(nil, "", true)
		if err != nil {
			return printUsageError(stderr, err.Error())
		}
		if len(ids) == 0 {
			return printUsageError(stderr, "no bookmark ids provided on stdin")
		}
	} else {
		if len(remaining) != 1 {
			return printUsageError(stderr, "usage: ip text <bookmark_id> [--out file] [--open]")
		}
		id, err := parseInt64(remaining[0])
		if err != nil {
			return printError(stderr, err)
		}
		ids = []int64{id}
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	if len(ids) > 1 && outPath == "" {
		return printUsageError(stderr, "text --stdin requires --out <directory> when multiple ids are provided")
	}
	if len(ids) > 1 {
		if err := os.MkdirAll(outPath, 0o700); err != nil {
			return printError(stderr, err)
		}
		for _, id := range ids {
			b, err := client.GetTextHTML(ctx, id)
			if err != nil {
				writeErrorLine(stderr, err)
				continue
			}
			path := filepath.Join(outPath, fmt.Sprintf("instapaper-%d.html", id))
			if err := os.WriteFile(path, b, 0o600); err != nil {
				writeErrorLine(stderr, err)
				continue
			}
			if !opts.Quiet {
				fmt.Fprintln(stdout, path)
			}
		}
		return 0
	}

	id := ids[0]
	b, err := client.GetTextHTML(ctx, id)
	if err != nil {
		return printError(stderr, err)
	}
	if outPath == "" {
		if openIt {
			outPath = filepath.Join(os.TempDir(), fmt.Sprintf("instapaper-%d.html", id))
		} else {
			_, _ = stdout.Write(b)
			return 0
		}
	}
	if err := os.WriteFile(outPath, b, 0o600); err != nil {
		return printError(stderr, err)
	}
	if !opts.Quiet {
		fmt.Fprintln(stdout, outPath)
	}
	if openIt {
		_ = browser.Open(outPath)
	}
	return 0
}

// --- folders ---
func runFolders(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageFolders())
		return 0
	}
	if len(args) == 0 {
		return printUsageError(stderr, "usage: ip folders list|add|delete|order")
	}
	sub := args[0]
	subArgs := args[1:]
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	switch sub {
	case "list":
		folders, err := client.ListFolders(ctx)
		if err != nil {
			return printError(stderr, err)
		}
		if err := output.PrintFolders(stdout, opts.Format, folders); err != nil {
			return printError(stderr, err)
		}
		return 0
	case "add":
		if hasHelpFlag(subArgs) {
			fmt.Fprintln(stdout, usageFoldersAdd())
			return 0
		}
		if len(subArgs) != 1 {
			return printUsageError(stderr, "usage: ip folders add \"Title\"")
		}
		if opts.DryRun {
			_ = emitDryRunAction(stdout, opts.Format, "folders.add", map[string]any{"title": subArgs[0]})
			return 0
		}
		f, err := client.AddFolder(ctx, subArgs[0])
		if err != nil {
			var apiErr *instapaper.APIError
			if opts.Idempotent && errors.As(err, &apiErr) && apiErr.Code == 1251 {
				if !opts.Quiet {
					fmt.Fprintln(stdout, "Folder already exists")
				}
				return 0
			}
			return printError(stderr, err)
		}
		if opts.Quiet {
			fmt.Fprintf(stdout, "%d\n", int64(f.FolderID))
			return 0
		}
		fmt.Fprintf(stdout, "Created folder %d: %s\n", int64(f.FolderID), f.Title)
		return 0
	case "delete":
		fs := flag.NewFlagSet("folders delete", flag.ContinueOnError)
		fs.SetOutput(stderr)
		subArgs = reorderFlags(subArgs)
		var help bool
		var yes bool
		var confirm string
		fs.BoolVar(&help, "help", false, "Show help")
		fs.BoolVar(&help, "h", false, "Show help")
		fs.BoolVar(&yes, "yes", false, "Confirm delete")
		fs.StringVar(&confirm, "confirm", "", "Confirm delete by repeating the folder id")
		if err := fs.Parse(subArgs); err != nil {
			return 2
		}
		if help {
			printFlagUsage(stdout, usageFoldersDelete(), fs)
			return 0
		}
		rest := fs.Args()
		if len(rest) != 1 {
			return printUsageError(stderr, "usage: ip folders delete <folder_id|\"Title\"> --yes|--confirm <folder_id>")
		}
		if !opts.DryRun && !yes && confirm == "" {
			return printUsageError(stderr, "refusing: folder delete requires --yes or --confirm <folder_id>")
		}
		folderIDStr, err := resolveUserFolderID(ctx, client, rest[0])
		if err != nil {
			return printError(stderr, err)
		}
		if folderIDStr == "" {
			return printUsageError(stderr, "must specify a user folder")
		}
		if confirm != "" && confirm != folderIDStr {
			return printError(stderr, fmt.Errorf("--confirm must match folder id"))
		}
		id, err := parseInt64(folderIDStr)
		if err != nil {
			return printError(stderr, err)
		}
		if opts.DryRun {
			_ = emitDryRunAction(stdout, opts.Format, "folders.delete", map[string]any{"folder_id": id})
			return 0
		}
		if err := client.DeleteFolder(ctx, id); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintf(stdout, "Deleted folder %d\n", id)
		}
		return 0
	case "order":
		if hasHelpFlag(subArgs) {
			fmt.Fprintln(stdout, usageFoldersOrder())
			return 0
		}
		if len(subArgs) != 1 {
			return printUsageError(stderr, "usage: ip folders order \"100:1,200:2,300:3\"")
		}
		if opts.DryRun {
			_ = emitDryRunAction(stdout, opts.Format, "folders.order", map[string]any{"order": subArgs[0]})
			return 0
		}
		folders, err := client.SetFolderOrder(ctx, subArgs[0])
		if err != nil {
			return printError(stderr, err)
		}
		if err := output.PrintFolders(stdout, opts.Format, folders); err != nil {
			return printError(stderr, err)
		}
		return 0
	default:
		return printUsageError(stderr, "usage: ip folders list|add|delete|order")
	}
}

// --- highlights ---
func runHighlights(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageHighlights())
		return 0
	}
	if len(args) == 0 {
		return printUsageError(stderr, "usage: ip highlights list|add|delete")
	}
	sub := args[0]
	subArgs := args[1:]
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	switch sub {
	case "list":
		if hasHelpFlag(subArgs) {
			fmt.Fprintln(stdout, usageHighlightsList())
			return 0
		}
		if len(subArgs) != 1 {
			return printUsageError(stderr, "usage: ip highlights list <bookmark_id>")
		}
		bid, err := parseInt64(subArgs[0])
		if err != nil {
			return printError(stderr, err)
		}
		hls, err := client.ListHighlights(ctx, bid)
		if err != nil {
			return printError(stderr, err)
		}
		if err := output.PrintHighlights(stdout, opts.Format, hls); err != nil {
			return printError(stderr, err)
		}
		return 0
	case "add":
		fs := flag.NewFlagSet("highlights add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		subArgs = reorderFlags(subArgs)
		var help bool
		var text string
		var position int
		fs.BoolVar(&help, "help", false, "Show help")
		fs.BoolVar(&help, "h", false, "Show help")
		fs.StringVar(&text, "text", "", "Highlight text")
		fs.IntVar(&position, "position", 0, "0-indexed position (optional)")
		if err := fs.Parse(subArgs); err != nil {
			return 2
		}
		if help {
			printFlagUsage(stdout, usageHighlightsAdd(), fs)
			return 0
		}
		rest := fs.Args()
		if len(rest) != 1 || strings.TrimSpace(text) == "" {
			return printUsageError(stderr, "usage: ip highlights add <bookmark_id> --text \"...\" [--position 0]")
		}
		bid, err := parseInt64(rest[0])
		if err != nil {
			return printError(stderr, err)
		}
		if opts.DryRun {
			_ = emitDryRunAction(stdout, opts.Format, "highlights.add", map[string]any{
				"bookmark_id": bid,
				"text":        text,
				"position":    position,
			})
			return 0
		}
		h, err := client.CreateHighlight(ctx, bid, text, position)
		if err != nil {
			if opts.Idempotent && isAlreadyStateError(err) {
				if !opts.Quiet {
					fmt.Fprintln(stdout, "Highlight already exists")
				}
				return 0
			}
			return printError(stderr, err)
		}
		if opts.Quiet {
			fmt.Fprintf(stdout, "%d\n", int64(h.HighlightID))
			return 0
		}
		fmt.Fprintf(stdout, "Created highlight %d\n", int64(h.HighlightID))
		return 0
	case "delete":
		if hasHelpFlag(subArgs) {
			fmt.Fprintln(stdout, usageHighlightsDelete())
			return 0
		}
		if len(subArgs) != 1 {
			return printUsageError(stderr, "usage: ip highlights delete <highlight_id>")
		}
		hid, err := parseInt64(subArgs[0])
		if err != nil {
			return printError(stderr, err)
		}
		if opts.DryRun {
			_ = emitDryRunAction(stdout, opts.Format, "highlights.delete", map[string]any{"highlight_id": hid})
			return 0
		}
		if err := client.DeleteHighlight(ctx, hid); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintf(stdout, "Deleted highlight %d\n", hid)
		}
		return 0
	default:
		return printUsageError(stderr, "usage: ip highlights list|add|delete")
	}
}

func validateFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "table", "plain", "json", "ndjson", "jsonl":
		return nil
	default:
		return fmt.Errorf("invalid --format %q (expected table, plain, json, or ndjson)", format)
	}
}

func printError(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	var apiErr *instapaper.APIError
	if errors.As(err, &apiErr) {
		hint := apiErrorHint(apiErr.Code)
		exitCode := exitCodeForError(apiErr)
		code := errorCodeForError(apiErr)
		if stderrJSONEnabled {
			payload := map[string]any{
				"error":     apiErr.Error(),
				"code":      code,
				"api_code":  apiErr.Code,
				"exit_code": exitCode,
			}
			if hint != "" {
				payload["hint"] = hint
			}
			_ = output.WriteJSONLine(stderr, payload)
			return exitCode
		}
		fmt.Fprintln(stderr, "error:", apiErr.Error())
		if hint != "" {
			fmt.Fprintln(stderr, "hint:", hint)
		}
		return exitCode
	}
	exitCode := exitCodeForError(err)
	code := errorCodeForError(err)
	if stderrJSONEnabled {
		payload := map[string]any{
			"error":     err.Error(),
			"code":      code,
			"exit_code": exitCode,
		}
		_ = output.WriteJSONLine(stderr, payload)
		return exitCode
	}
	fmt.Fprintln(stderr, "error:", err)
	return exitCode
}

func printUsageError(stderr io.Writer, msg string) int {
	if stderrJSONEnabled {
		payload := map[string]any{
			"error":     msg,
			"code":      ErrCodeInvalidUsage,
			"exit_code": 2,
		}
		_ = output.WriteJSONLine(stderr, payload)
		return 2
	}
	fmt.Fprintln(stderr, "error:", msg)
	return 2
}

func writeErrorLine(stderr io.Writer, err error) {
	if stderrJSONEnabled {
		_ = output.WriteJSONLine(stderr, map[string]any{
			"error": err.Error(),
			"code":  errorCodeForError(err),
		})
		return
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
}

func exitCodeForError(err error) int {
	var apiErr *instapaper.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 1040:
			return 10 // rate limited
		case 1041:
			return 11 // premium required
		case 1042:
			return 12 // application suspended
		case 1240, 1241, 1242, 1243, 1244, 1245, 1250, 1251, 1252, 1600, 1601, 1220, 1221:
			return 13 // invalid request
		case 1500, 1550:
			return 14 // server error
		default:
			return 1
		}
	}
	return 1
}

func apiErrorHint(code int) string {
	switch code {
	case 1040:
		return "rate limit exceeded; wait and retry"
	case 1041:
		return "requires Instapaper Premium"
	case 1042:
		return "application suspended; check Instapaper API status"
	case 1220:
		return "supply content for this bookmark"
	case 1221:
		return "the URL is not available from this source"
	case 1240:
		return "invalid URL"
	case 1241:
		return "invalid bookmark ID"
	case 1242:
		return "invalid folder ID"
	case 1243:
		return "invalid progress value"
	case 1244:
		return "invalid progress timestamp"
	case 1245:
		return "private source requires supplied content"
	case 1250:
		return "invalid title or unexpected error saving bookmark"
	case 1251:
		return "folder already exists"
	case 1252:
		return "cannot add bookmarks to this folder"
	case 1500:
		return "temporary service error; retry later"
	case 1550:
		return "text view generation error; retry later"
	case 1600:
		return "highlight text is required"
	case 1601:
		return "duplicate highlight"
	default:
		return ""
	}
}

func isAlreadyStateError(err error) bool {
	var apiErr *instapaper.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Code == 1601 {
			return true
		}
		if strings.Contains(strings.ToLower(apiErr.Message), "already") {
			return true
		}
	}
	return false
}

func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func printFlagUsage(w io.Writer, usage string, fs *flag.FlagSet) {
	fmt.Fprintln(w, usage)
	if fs == nil {
		return
	}
	fmt.Fprintln(w, "\nFlags:")
	fs.SetOutput(w)
	fs.PrintDefaults()
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func openOutputWriter(outputPath string, stdout io.Writer) (io.Writer, func(), error) {
	if outputPath == "" || outputPath == "-" {
		return stdout, nil, nil
	}
	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func loadCursor(path string) (*listCursor, error) {
	cur := &listCursor{Have: map[string]cursorEntry{}}
	if path == "" {
		return cur, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cur, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return cur, nil
	}
	if err := json.Unmarshal(b, cur); err != nil {
		return nil, fmt.Errorf("parse cursor %s: %w", path, err)
	}
	if cur.Have == nil {
		cur.Have = map[string]cursorEntry{}
	}
	return cur, nil
}

func saveCursor(path string, cur *listCursor) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func updateCursor(cur *listCursor, bookmarks []instapaper.Bookmark, deleteIDs []instapaper.Int64) {
	if cur == nil {
		return
	}
	for _, b := range bookmarks {
		id := strconv.FormatInt(int64(b.BookmarkID), 10)
		entry := cursorEntry{
			Hash:              b.Hash,
			Progress:          float64(b.Progress),
			ProgressTimestamp: int64(b.ProgressTimestamp),
		}
		cur.Have[id] = entry
	}
	for _, id := range deleteIDs {
		delete(cur.Have, strconv.FormatInt(int64(id), 10))
	}
}

func haveStringFromCursor(cur *listCursor) string {
	if cur == nil || len(cur.Have) == 0 {
		return ""
	}
	ids := make([]string, 0, len(cur.Have))
	for id := range cur.Have {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		entry := cur.Have[id]
		parts = append(parts, formatHaveEntry(id, entry))
	}
	return strings.Join(parts, ",")
}

func formatHaveEntry(id string, entry cursorEntry) string {
	if entry.Hash == "" {
		return id
	}
	if entry.ProgressTimestamp > 0 {
		return strings.Join([]string{
			id,
			entry.Hash,
			strconv.FormatFloat(entry.Progress, 'f', -1, 64),
			strconv.FormatInt(entry.ProgressTimestamp, 10),
		}, ":")
	}
	return id + ":" + entry.Hash
}

func mergeHaveString(cur *listCursor, have string) {
	if cur == nil {
		return
	}
	for _, part := range strings.Split(have, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Split(part, ":")
		id := strings.TrimSpace(fields[0])
		if id == "" {
			continue
		}
		entry := cursorEntry{}
		if len(fields) > 1 {
			entry.Hash = fields[1]
		}
		if len(fields) > 3 {
			if p, err := strconv.ParseFloat(fields[2], 64); err == nil {
				entry.Progress = p
			}
			if ts, err := strconv.ParseInt(fields[3], 10, 64); err == nil {
				entry.ProgressTimestamp = ts
			}
		}
		cur.Have[id] = entry
	}
}

type boundSpec struct {
	Field string
	Value int64
}

func parseBoundSpec(spec string, defaultField string) (*boundSpec, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	field := strings.ToLower(strings.TrimSpace(defaultField))
	valuePart := spec
	if idx := strings.Index(spec, ":"); idx > -1 {
		field = strings.ToLower(strings.TrimSpace(spec[:idx]))
		valuePart = strings.TrimSpace(spec[idx+1:])
		if field == "" {
			field = strings.ToLower(strings.TrimSpace(defaultField))
		}
	}
	if valuePart == "" {
		return nil, fmt.Errorf("invalid bound %q", spec)
	}
	field = normalizeBoundField(field)
	switch field {
	case "bookmark_id":
		v, err := parseInt64(valuePart)
		if err != nil {
			return nil, err
		}
		return &boundSpec{Field: field, Value: v}, nil
	case "time", "progress_timestamp", "updated":
		v, err := parseTimeValue(valuePart)
		if err != nil {
			return nil, err
		}
		return &boundSpec{Field: field, Value: v}, nil
	default:
		return nil, fmt.Errorf("unknown bound field: %s", field)
	}
}

func parseUpdatedBound(spec string) (*boundSpec, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	v, err := parseTimeValue(spec)
	if err != nil {
		return nil, err
	}
	return &boundSpec{Field: "updated", Value: v}, nil
}

func normalizeBoundField(field string) string {
	switch field {
	case "id", "bookmark", "bookmarkid", "bookmark_id":
		return "bookmark_id"
	case "time", "created", "created_at":
		return "time"
	case "progress", "progress_ts", "progress_timestamp":
		return "progress_timestamp"
	case "updated", "updated_at":
		return "updated"
	default:
		return field
	}
}

func parseTimeValue(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("invalid time value")
	}
	if v, err := strconv.ParseInt(value, 10, 64); err == nil {
		return v, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.Unix(), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Unix(), nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("invalid time value: %s", value)
}

func filterBookmarksByBounds(bookmarks []instapaper.Bookmark, since, until *boundSpec) []instapaper.Bookmark {
	if since == nil && until == nil {
		return bookmarks
	}
	out := make([]instapaper.Bookmark, 0, len(bookmarks))
	for _, b := range bookmarks {
		if !bookmarkWithinBounds(b, since, until) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func bookmarkWithinBounds(b instapaper.Bookmark, since, until *boundSpec) bool {
	if since != nil {
		if bookmarkFieldValue(b, since.Field) < since.Value {
			return false
		}
	}
	if until != nil {
		if bookmarkFieldValue(b, until.Field) > until.Value {
			return false
		}
	}
	return true
}

func bookmarkFieldValue(b instapaper.Bookmark, field string) int64 {
	switch field {
	case "bookmark_id":
		return int64(b.BookmarkID)
	case "progress_timestamp":
		return int64(b.ProgressTimestamp)
	case "updated":
		return updatedValue(b)
	default:
		return int64(b.Time)
	}
}

func updatedValue(b instapaper.Bookmark) int64 {
	if b.ProgressTimestamp > 0 {
		return int64(b.ProgressTimestamp)
	}
	return int64(b.Time)
}

func resolveCursorPath(dir, folderID, tag string) string {
	if dir == "" {
		return ""
	}
	name := "unread"
	if tag != "" {
		name = "tag-" + tag
	} else if folderID != "" {
		name = "folder-" + folderID
	}
	name = sanitizeFilename(name)
	return filepath.Join(dir, name+".json")
}

func sanitizeFilename(name string) string {
	if name == "" {
		return "cursor"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "cursor"
	}
	return out
}

type pagedExportWriter struct {
	dir    string
	prefix string
	fields string
	pages  int
}

func newPagedExportWriter(dir, folderID, tag, fields string) (*pagedExportWriter, error) {
	if dir == "" {
		return nil, fmt.Errorf("output dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name := exportTargetName(folderID, tag)
	return &pagedExportWriter{
		dir:    dir,
		prefix: sanitizeFilename(name),
		fields: fields,
	}, nil
}

func (w *pagedExportWriter) WritePage(pageIndex int, bookmarks []instapaper.Bookmark) error {
	if len(bookmarks) == 0 {
		return nil
	}
	filename := fmt.Sprintf("%s-%04d.ndjson", w.prefix, pageIndex)
	path := filepath.Join(w.dir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if w.fields != "" {
		if err := output.PrintBookmarksWithFields(f, "ndjson", bookmarks, w.fields); err != nil {
			return err
		}
	} else {
		if err := output.PrintBookmarks(f, "ndjson", bookmarks); err != nil {
			return err
		}
	}
	w.pages++
	return nil
}

func exportTargetName(folderID, tag string) string {
	if tag != "" {
		return "tag-" + tag
	}
	if folderID != "" {
		return "folder-" + folderID
	}
	return "unread"
}

type progressEmitter struct {
	enabled bool
	writer  io.Writer
	action  string
	total   int
	current int
	success int
	failed  int
}

func newProgressEmitter(enabled bool, w io.Writer, action string, total int) *progressEmitter {
	return &progressEmitter{
		enabled: enabled,
		writer:  w,
		action:  action,
		total:   total,
	}
}

func (p *progressEmitter) Start() {
	if !p.enabled {
		return
	}
	payload := map[string]any{
		"event":  "start",
		"action": p.action,
	}
	if p.total > 0 {
		payload["total"] = p.total
	}
	_ = output.WriteJSONLine(p.writer, payload)
}

func (p *progressEmitter) ItemSuccess(meta map[string]any) {
	if !p.enabled {
		return
	}
	p.current++
	p.success++
	payload := map[string]any{
		"event":   "item",
		"action":  p.action,
		"status":  "ok",
		"current": p.current,
		"success": p.success,
		"failed":  p.failed,
		"data":    meta,
	}
	if p.total > 0 {
		payload["total"] = p.total
	}
	_ = output.WriteJSONLine(p.writer, payload)
}

func (p *progressEmitter) ItemError(meta map[string]any, err error) {
	if !p.enabled {
		return
	}
	p.current++
	p.failed++
	payload := map[string]any{
		"event":   "item",
		"action":  p.action,
		"status":  "error",
		"error":   err.Error(),
		"current": p.current,
		"success": p.success,
		"failed":  p.failed,
		"data":    meta,
	}
	if p.total > 0 {
		payload["total"] = p.total
	}
	_ = output.WriteJSONLine(p.writer, payload)
}

func (p *progressEmitter) Done() {
	if !p.enabled {
		return
	}
	payload := map[string]any{
		"event":   "done",
		"action":  p.action,
		"success": p.success,
		"failed":  p.failed,
	}
	if p.total > 0 {
		payload["total"] = p.total
	}
	_ = output.WriteJSONLine(p.writer, payload)
}

func collectIDs(args []string, idsCSV string, stdin bool) ([]int64, error) {
	if idsCSV != "" && stdin {
		return nil, fmt.Errorf("use only one of --ids or --stdin")
	}
	if idsCSV != "" {
		return parseIDList(idsCSV)
	}
	if stdin {
		return readIDsFromReader(os.Stdin)
	}
	if len(args) == 0 {
		return nil, nil
	}
	return parseIDList(strings.Join(args, ","))
}

func parseIDList(value string) ([]int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t' || r == ' '
	})
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := parseInt64(part)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func readIDsFromReader(r io.Reader) ([]int64, error) {
	scanner := bufio.NewScanner(r)
	var ids []int64
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		id, err := parseInt64(line)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func emitDryRunIDs(w io.Writer, format, action string, ids []int64) int {
	if strings.EqualFold(format, "json") {
		payload := map[string]any{
			"dry_run": true,
			"action":  action,
			"ids":     ids,
		}
		if err := output.WriteJSON(w, payload); err != nil {
			return 1
		}
		return 0
	}
	if isNDJSONFormat(format) {
		for _, id := range ids {
			if err := output.WriteJSONLine(w, map[string]any{
				"dry_run": true,
				"action":  action,
				"data": map[string]any{
					"bookmark_id": id,
				},
			}); err != nil {
				return 1
			}
		}
		return 0
	}
	for _, id := range ids {
		fmt.Fprintf(w, "DRY RUN: %s %d\n", action, id)
	}
	return 0
}

func verbosef(opts *GlobalOptions, stderr io.Writer, format string, args ...any) {
	if opts == nil || !opts.Verbose || opts.Quiet {
		return
	}
	fmt.Fprintf(stderr, format+"\n", args...)
}

type selectFilter struct {
	Field string
	Op    string
	Value string
}

func filterBookmarksBySelect(bookmarks []instapaper.Bookmark, expr string) ([]instapaper.Bookmark, error) {
	filters, err := parseSelectExpr(expr)
	if err != nil {
		return nil, err
	}
	return filterBookmarksBySelectFilters(bookmarks, filters), nil
}

func filterBookmarksBySelectFilters(bookmarks []instapaper.Bookmark, filters []selectFilter) []instapaper.Bookmark {
	if len(filters) == 0 {
		return bookmarks
	}
	out := make([]instapaper.Bookmark, 0, len(bookmarks))
	for _, b := range bookmarks {
		if matchSelectFilters(b, filters) {
			out = append(out, b)
		}
	}
	return out
}

func parseSelectExpr(expr string) ([]selectFilter, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}
	parts := strings.Split(expr, ",")
	filters := make([]selectFilter, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		field, op, value, err := splitFilter(part)
		if err != nil {
			return nil, err
		}
		filter := selectFilter{Field: field, Op: op, Value: value}
		if err := validateSelectFilter(filter); err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

func splitFilter(expr string) (string, string, string, error) {
	var op string
	switch {
	case strings.Contains(expr, "!="):
		op = "!="
	case strings.Contains(expr, "~"):
		op = "~"
	case strings.Contains(expr, "="):
		op = "="
	default:
		return "", "", "", fmt.Errorf("invalid select filter: %s", expr)
	}
	parts := strings.SplitN(expr, op, 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid select filter: %s", expr)
	}
	field := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])
	if field == "" || value == "" {
		return "", "", "", fmt.Errorf("invalid select filter: %s", expr)
	}
	field = normalizeSelectField(field)
	return field, op, value, nil
}

func normalizeSelectField(field string) string {
	switch field {
	case "id", "bookmark", "bookmarkid", "bookmark_id":
		return "bookmark_id"
	case "progress_ts", "progress_timestamp":
		return "progress_timestamp"
	case "tag", "tags":
		return "tags"
	case "star", "starred":
		return "starred"
	default:
		return field
	}
}

func validateSelectFilter(f selectFilter) error {
	switch f.Field {
	case "bookmark_id", "time", "progress_timestamp":
		if f.Op != "=" && f.Op != "!=" {
			return fmt.Errorf("unsupported operator for %s: %s", f.Field, f.Op)
		}
		if _, err := strconv.ParseInt(f.Value, 10, 64); err != nil {
			return fmt.Errorf("invalid numeric value for %s: %s", f.Field, f.Value)
		}
	case "progress":
		if f.Op != "=" && f.Op != "!=" {
			return fmt.Errorf("unsupported operator for %s: %s", f.Field, f.Op)
		}
		if _, err := strconv.ParseFloat(f.Value, 64); err != nil {
			return fmt.Errorf("invalid numeric value for %s: %s", f.Field, f.Value)
		}
	case "starred":
		if f.Op != "=" && f.Op != "!=" {
			return fmt.Errorf("unsupported operator for %s: %s", f.Field, f.Op)
		}
		if _, err := parseBool(f.Value); err != nil {
			return fmt.Errorf("invalid boolean value for %s: %s", f.Field, f.Value)
		}
	case "title", "url", "description", "tags":
		if f.Op != "=" && f.Op != "!=" && f.Op != "~" {
			return fmt.Errorf("unsupported operator for %s: %s", f.Field, f.Op)
		}
	default:
		return fmt.Errorf("unknown select field: %s", f.Field)
	}
	return nil
}

func matchSelectFilters(b instapaper.Bookmark, filters []selectFilter) bool {
	for _, f := range filters {
		if !matchSelectFilter(b, f) {
			return false
		}
	}
	return true
}

func matchSelectFilter(b instapaper.Bookmark, f selectFilter) bool {
	switch f.Field {
	case "bookmark_id":
		return matchInt64(int64(b.BookmarkID), f)
	case "time":
		return matchInt64(int64(b.Time), f)
	case "progress_timestamp":
		return matchInt64(int64(b.ProgressTimestamp), f)
	case "progress":
		return matchFloat64(float64(b.Progress), f)
	case "starred":
		return matchBool(bool(b.Starred), f)
	case "title":
		return matchString(b.Title, f)
	case "url":
		return matchString(b.URL, f)
	case "description":
		return matchString(b.Description, f)
	case "tags":
		return matchTags(b.Tags, f)
	default:
		return false
	}
}

func matchInt64(value int64, f selectFilter) bool {
	v, err := strconv.ParseInt(f.Value, 10, 64)
	if err != nil {
		return false
	}
	switch f.Op {
	case "=":
		return value == v
	case "!=":
		return value != v
	default:
		return false
	}
}

func matchFloat64(value float64, f selectFilter) bool {
	v, err := strconv.ParseFloat(f.Value, 64)
	if err != nil {
		return false
	}
	switch f.Op {
	case "=":
		return value == v
	case "!=":
		return value != v
	default:
		return false
	}
}

func matchBool(value bool, f selectFilter) bool {
	v, err := parseBool(f.Value)
	if err != nil {
		return false
	}
	switch f.Op {
	case "=":
		return value == v
	case "!=":
		return value != v
	default:
		return false
	}
}

func matchString(value string, f selectFilter) bool {
	switch f.Op {
	case "=":
		return strings.EqualFold(value, f.Value)
	case "!=":
		return !strings.EqualFold(value, f.Value)
	case "~":
		return strings.Contains(strings.ToLower(value), strings.ToLower(f.Value))
	default:
		return false
	}
}

func matchTags(tags []instapaper.Tag, f selectFilter) bool {
	for _, tag := range tags {
		switch f.Op {
		case "=":
			if strings.EqualFold(tag.Name, f.Value) {
				return true
			}
		case "!=":
			if strings.EqualFold(tag.Name, f.Value) {
				return false
			}
		case "~":
			if strings.Contains(strings.ToLower(tag.Name), strings.ToLower(f.Value)) {
				return true
			}
		}
	}
	return f.Op == "!="
}

func printConfig(w io.Writer, cfg *config.Config) error {
	tw := tabwriter.NewWriter(w, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintf(tw, "api_base\t%s\n", cfg.APIBase)
	if cfg.ConsumerKey != "" {
		fmt.Fprintf(tw, "consumer_key\t%s\n", cfg.ConsumerKey)
	}
	if cfg.ConsumerSecret != "" {
		fmt.Fprintf(tw, "consumer_secret\t%s\n", cfg.ConsumerSecret)
	}
	fmt.Fprintf(tw, "defaults.format\t%s\n", cfg.Defaults.Format)
	fmt.Fprintf(tw, "defaults.list_limit\t%d\n", cfg.Defaults.ListLimit)
	if cfg.Defaults.ResolveFinalURL != nil {
		fmt.Fprintf(tw, "defaults.resolve_final_url\t%t\n", *cfg.Defaults.ResolveFinalURL)
	}
	if cfg.HasAuth() {
		fmt.Fprintf(tw, "user.user_id\t%d\n", cfg.User.UserID)
		fmt.Fprintf(tw, "user.username\t%s\n", cfg.User.Username)
	}
	return tw.Flush()
}

func readImportItems(path, format, defaultFolder, defaultTags string, defaultArchive bool) ([]importItem, error) {
	r, closeFn, err := openInputReader(path)
	if err != nil {
		return nil, err
	}
	if closeFn != nil {
		defer closeFn()
	}
	defaultTagList := splitTags(defaultTags)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "plain":
		return readPlainImportItems(r, defaultFolder, defaultTagList, defaultArchive)
	case "csv":
		return readCSVImportItems(r, defaultFolder, defaultTagList, defaultArchive)
	case "ndjson", "jsonl":
		return readNDJSONImportItems(r, defaultFolder, defaultTagList, defaultArchive)
	default:
		return nil, fmt.Errorf("invalid --input-format %q (expected plain, csv, or ndjson)", format)
	}
}

func openInputReader(path string) (io.Reader, func(), error) {
	if path == "" || path == "-" {
		return os.Stdin, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func readPlainImportItems(r io.Reader, folder string, tags []string, archive bool) ([]importItem, error) {
	var items []importItem
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		items = append(items, importItem{
			URL:     line,
			Folder:  folder,
			Tags:    tags,
			Archive: archive,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func readNDJSONImportItems(r io.Reader, folder string, tags []string, archive bool) ([]importItem, error) {
	var items []importItem
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, fmt.Errorf("parse ndjson: %w", err)
		}
		url := toString(obj["url"])
		if url == "" {
			return nil, errors.New("missing url in ndjson item")
		}
		item := importItem{
			URL:         url,
			Title:       toString(obj["title"]),
			Description: toString(obj["description"]),
			Folder:      toString(obj["folder"]),
			Tags:        parseTagsValue(obj["tags"]),
			Archive:     toBool(obj["archive"], archive),
		}
		if item.Folder == "" {
			item.Folder = folder
		}
		item.Tags = mergeTags(item.Tags, tags)
		items = append(items, item)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func readCSVImportItems(r io.Reader, folder string, tags []string, archive bool) ([]importItem, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	header := map[string]int{}
	start := 0
	for i, col := range rows[0] {
		name := strings.ToLower(strings.TrimSpace(col))
		if name != "" {
			header[name] = i
		}
	}
	if _, ok := header["url"]; ok {
		start = 1
	} else {
		header = map[string]int{
			"url":   0,
			"title": 1,
			"tags":  2,
		}
	}
	items := make([]importItem, 0, len(rows)-start)
	for _, row := range rows[start:] {
		url := getCSV(row, header, "url")
		if strings.TrimSpace(url) == "" {
			continue
		}
		item := importItem{
			URL:         url,
			Title:       getCSV(row, header, "title"),
			Description: getCSV(row, header, "description"),
			Folder:      getCSV(row, header, "folder"),
			Tags:        splitTags(getCSV(row, header, "tags")),
			Archive:     toBool(getCSV(row, header, "archive"), archive),
		}
		if item.Folder == "" {
			item.Folder = folder
		}
		item.Tags = mergeTags(item.Tags, tags)
		items = append(items, item)
	}
	return items, nil
}

func getCSV(row []string, header map[string]int, key string) string {
	idx, ok := header[key]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func toBool(v any, fallback bool) bool {
	if v == nil {
		return fallback
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		b, err := parseBool(t)
		if err != nil {
			return fallback
		}
		return b
	default:
		return fallback
	}
}

func parseTagsValue(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case string:
		return splitTags(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := toString(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func splitTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mergeTags(primary, fallback []string) []string {
	if len(primary) == 0 {
		return fallback
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(primary)+len(fallback))
	for _, t := range primary {
		if !seen[t] {
			out = append(out, t)
			seen[t] = true
		}
	}
	for _, t := range fallback {
		if !seen[t] {
			out = append(out, t)
			seen[t] = true
		}
	}
	return out
}

func emitDryRunAction(w io.Writer, format, action string, data map[string]any) error {
	payload := map[string]any{
		"dry_run": true,
		"action":  action,
		"data":    data,
	}
	if strings.EqualFold(format, "json") {
		return output.WriteJSON(w, payload)
	}
	if isNDJSONFormat(format) {
		return output.WriteJSONLine(w, payload)
	}
	fmt.Fprintf(w, "DRY RUN: %s\n", action)
	return nil
}

func emitDryRunItems(w io.Writer, format, action string, items []importItem) int {
	records := make([]map[string]any, 0, len(items))
	for _, item := range items {
		records = append(records, map[string]any{
			"url":         item.URL,
			"title":       item.Title,
			"description": item.Description,
			"tags":        item.Tags,
			"folder":      item.Folder,
			"archive":     item.Archive,
		})
	}
	return emitDryRunRecords(w, format, action, records)
}

func emitDryRunRecords(w io.Writer, format, action string, records []map[string]any) int {
	if strings.EqualFold(format, "json") {
		payload := map[string]any{
			"dry_run": true,
			"action":  action,
			"items":   records,
		}
		if err := output.WriteJSON(w, payload); err != nil {
			return 1
		}
		return 0
	}
	if isNDJSONFormat(format) {
		for _, record := range records {
			if err := output.WriteJSONLine(w, map[string]any{
				"dry_run": true,
				"action":  action,
				"data":    record,
			}); err != nil {
				return 1
			}
		}
		return 0
	}
	for _, record := range records {
		url, _ := record["url"].(string)
		fmt.Fprintf(w, "DRY RUN: %s %s\n", action, url)
	}
	return 0
}

func printConfigPlain(w io.Writer, cfg *config.Config) error {
	fmt.Fprintf(w, "api_base=%s\n", cfg.APIBase)
	if cfg.ConsumerKey != "" {
		fmt.Fprintf(w, "consumer_key=%s\n", cfg.ConsumerKey)
	}
	if cfg.ConsumerSecret != "" {
		fmt.Fprintf(w, "consumer_secret=%s\n", cfg.ConsumerSecret)
	}
	fmt.Fprintf(w, "defaults.format=%s\n", cfg.Defaults.Format)
	fmt.Fprintf(w, "defaults.list_limit=%d\n", cfg.Defaults.ListLimit)
	if cfg.Defaults.ResolveFinalURL != nil {
		fmt.Fprintf(w, "defaults.resolve_final_url=%t\n", *cfg.Defaults.ResolveFinalURL)
	}
	if cfg.HasAuth() {
		fmt.Fprintf(w, "user.user_id=%d\n", cfg.User.UserID)
		fmt.Fprintf(w, "user.username=%s\n", cfg.User.Username)
	}
	return nil
}

func configGet(cfg *config.Config, key string) (any, bool, error) {
	switch key {
	case "api_base":
		return cfg.APIBase, true, nil
	case "consumer_key":
		return cfg.ConsumerKey, true, nil
	case "consumer_secret":
		return cfg.ConsumerSecret, true, nil
	case "defaults.format":
		return cfg.Defaults.Format, true, nil
	case "defaults.list_limit":
		return cfg.Defaults.ListLimit, true, nil
	case "defaults.resolve_final_url":
		if cfg.Defaults.ResolveFinalURL == nil {
			return nil, true, nil
		}
		return *cfg.Defaults.ResolveFinalURL, true, nil
	default:
		return nil, false, nil
	}
}

func configSet(cfg *config.Config, key, value string) error {
	switch key {
	case "api_base":
		cfg.APIBase = value
		return nil
	case "consumer_key":
		cfg.ConsumerKey = value
		return nil
	case "consumer_secret":
		cfg.ConsumerSecret = value
		return nil
	case "defaults.format":
		if err := validateFormat(value); err != nil {
			return err
		}
		cfg.Defaults.Format = value
		return nil
	case "defaults.list_limit":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid list_limit: %w", err)
		}
		if v < 0 || v > 500 {
			return fmt.Errorf("invalid list_limit %d (expected 0..500)", v)
		}
		cfg.Defaults.ListLimit = v
		return nil
	case "defaults.resolve_final_url":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Defaults.ResolveFinalURL = &b
		return nil
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
}

func configUnset(cfg *config.Config, key string) error {
	switch key {
	case "api_base":
		cfg.APIBase = ""
	case "consumer_key":
		cfg.ConsumerKey = ""
	case "consumer_secret":
		cfg.ConsumerSecret = ""
	case "defaults.format":
		cfg.Defaults.Format = ""
	case "defaults.list_limit":
		cfg.Defaults.ListLimit = 0
	case "defaults.resolve_final_url":
		cfg.Defaults.ResolveFinalURL = nil
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean: %s", value)
	}
}

func isNDJSONFormat(format string) bool {
	return strings.EqualFold(format, "ndjson") || strings.EqualFold(format, "jsonl")
}

func writeJSONByFormat(w io.Writer, format string, v any) error {
	if isNDJSONFormat(format) {
		return output.WriteJSONLine(w, v)
	}
	return output.WriteJSON(w, v)
}

func schemaForTarget(target string) (map[string]any, bool) {
	base := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
	}
	switch target {
	case "bookmarks", "bookmark":
		base["type"] = "object"
		base["properties"] = map[string]any{
			"type":               map[string]any{"type": "string"},
			"bookmark_id":        map[string]any{"type": "integer"},
			"url":                map[string]any{"type": "string"},
			"title":              map[string]any{"type": "string"},
			"description":        map[string]any{"type": "string"},
			"hash":               map[string]any{"type": "string"},
			"progress":           map[string]any{"type": "number"},
			"progress_timestamp": map[string]any{"type": "integer"},
			"starred":            map[string]any{"type": "boolean"},
			"private_source":     map[string]any{"type": "string"},
			"time":               map[string]any{"type": "integer"},
			"tags":               map[string]any{"type": "array"},
		}
		return base, true
	case "folders", "folder":
		base["type"] = "object"
		base["properties"] = map[string]any{
			"type":      map[string]any{"type": "string"},
			"folder_id": map[string]any{"type": "integer"},
			"title":     map[string]any{"type": "string"},
			"position":  map[string]any{"type": "integer"},
		}
		return base, true
	case "highlights", "highlight":
		base["type"] = "object"
		base["properties"] = map[string]any{
			"type":         map[string]any{"type": "string"},
			"highlight_id": map[string]any{"type": "integer"},
			"bookmark_id":  map[string]any{"type": "integer"},
			"text":         map[string]any{"type": "string"},
			"time":         map[string]any{"type": "integer"},
			"position":     map[string]any{"type": "integer"},
		}
		return base, true
	case "auth":
		base["type"] = "object"
		base["properties"] = map[string]any{
			"logged_in": map[string]any{"type": "boolean"},
			"user": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"user_id":  map[string]any{"type": "integer"},
					"username": map[string]any{"type": "string"},
				},
			},
		}
		return base, true
	case "config":
		base["type"] = "object"
		base["properties"] = map[string]any{
			"api_base":        map[string]any{"type": "string"},
			"consumer_key":    map[string]any{"type": "string"},
			"consumer_secret": map[string]any{"type": "string"},
			"defaults":        map[string]any{"type": "object"},
			"user":            map[string]any{"type": "object"},
		}
		return base, true
	default:
		return nil, false
	}
}

func usageConfig() string {
	return "Usage:\n  ip config path|show|get|set|unset\n"
}

func usageAuth() string {
	return "Usage:\n  ip auth login|status|logout\n"
}

func usageAuthLogin() string {
	return "Usage:\n  ip auth login [flags]\n"
}

func usageAdd() string {
	return "Usage:\n  ip add <url|-> [flags]\n"
}

func usageList() string {
	return "Usage:\n  ip list [--folder ...] [--limit N] [--tag name] [--have ...] [--highlights ...] [--fields ...] [--cursor <file>] [--cursor-dir <dir>] [--since <bound>] [--until <bound>] [--updated-since <time>] [--max-pages N] [--select <expr>]\n"
}

func usageExport() string {
	return "Usage:\n  ip export [--folder ...] [--tag ...] [--limit N] [--fields ...] [--cursor <file>] [--cursor-dir <dir>] [--since <bound>] [--until <bound>] [--updated-since <time>] [--max-pages N] [--select <expr>] [--output-dir <dir>]\n"
}

func usageImport() string {
	return "Usage:\n  ip import [--input <file>|-] [--input-format plain|csv|ndjson] [--folder ...] [--tags ...] [--archive] [--progress-json]\n"
}

func usageBookmarkMutation(cmd string) string {
	return fmt.Sprintf("Usage:\n  ip %s <bookmark_id> [<bookmark_id> ...] [--ids <ids>] [--stdin] [--batch N] [--progress-json]\n", cmd)
}

func usageMove() string {
	return "Usage:\n  ip move --folder <folder_id|\"Title\"> <bookmark_id>\n"
}

func usageDelete() string {
	return "Usage:\n  ip delete <bookmark_id> [--ids <ids>] [--stdin] [--batch N] [--progress-json] --yes-really-delete|--confirm <bookmark_id>\n"
}

func usageProgress() string {
	return "Usage:\n  ip progress <bookmark_id> --progress <0..1> --timestamp <unix>\n"
}

func usageText() string {
	return "Usage:\n  ip text <bookmark_id> [--out <file>] [--open]\n  ip text --stdin --out <dir>\n"
}

func usageFolders() string {
	return "Usage:\n  ip folders list|add|delete|order\n"
}

func usageFoldersAdd() string {
	return "Usage:\n  ip folders add \"Title\"\n"
}

func usageFoldersDelete() string {
	return "Usage:\n  ip folders delete <folder_id|\"Title\"> --yes|--confirm <folder_id>\n"
}

func usageFoldersOrder() string {
	return "Usage:\n  ip folders order \"100:1,200:2,300:3\"\n"
}

func usageHighlights() string {
	return "Usage:\n  ip highlights list|add|delete\n"
}

func usageHighlightsList() string {
	return "Usage:\n  ip highlights list <bookmark_id>\n"
}

func usageHighlightsAdd() string {
	return "Usage:\n  ip highlights add <bookmark_id> --text \"...\" [--position 0]\n"
}

func usageHighlightsDelete() string {
	return "Usage:\n  ip highlights delete <highlight_id>\n"
}

func usageHealth() string {
	return "Usage:\n  ip health\n"
}

func usageDoctor() string {
	return "Usage:\n  ip doctor\n"
}

func usageVerify() string {
	return "Usage:\n  ip verify\n"
}

func usageSchema() string {
	return "Usage:\n  ip schema [bookmarks|folders|highlights|auth|config]\n"
}

func usageTags() string {
	return "Usage:\n  ip tags list|rename|delete\n"
}

func usageAgent() string {
	return `AI agent tips:
  - Default output is NDJSON; override with --format table|plain|json if needed.
  - Use --ndjson/--jsonl for streams, --json for single objects.
  - Prefer --plain only for line-oriented, human-friendly output.
  - Use --stderr-json for structured errors, codes, and exit codes.
  - For deterministic output, avoid table mode.
  - Run ip doctor before long workflows to validate config/auth/network.
  - Use --since/--until or --updated-since to slice lists without cursors.
  - Use --cursor-dir for automatic incremental sync files.
  - Use --ids or --stdin for bulk mutations; add --progress-json for progress events.
  - Use --select to client-filter results when API filters are missing.
Examples:
  ip --json auth status
  ip doctor --json
  ip list --ndjson --limit 0 --max-pages 50
  ip list --updated-since 2025-01-01T00:00:00Z
  ip list --select "starred=1,tag~news"
  ip list --plain --output bookmarks.txt
`
}
