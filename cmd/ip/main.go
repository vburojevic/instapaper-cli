package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	ConfigPath string
	Format     string
	Quiet      bool
	Debug      bool
	Timeout    time.Duration
	APIBase    string
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
	global.StringVar(&opts.ConfigPath, "config", "", "Path to config file (default: user config dir)")
	global.StringVar(&opts.Format, "format", "", "Output format: table, plain, or json")
	global.BoolVar(&opts.Quiet, "quiet", false, "Less output")
	global.BoolVar(&opts.Debug, "debug", false, "Debug output (never prints secrets)")
	global.DurationVar(&opts.Timeout, "timeout", 15*time.Second, "HTTP timeout")
	global.StringVar(&opts.APIBase, "api-base", "", "API base URL (default: https://www.instapaper.com)")
	global.BoolVar(&jsonOutput, "json", false, "Output JSON (alias for --format json)")
	global.BoolVar(&plainOutput, "plain", false, "Output plain text (alias for --format plain)")
	global.BoolVar(&ndjsonOutput, "ndjson", false, "Output NDJSON (alias for --format ndjson)")
	global.BoolVar(&jsonlOutput, "jsonl", false, "Output NDJSON (alias for --format ndjson)")
	global.BoolVar(&showVersion, "version", false, "Show version")
	global.BoolVar(&help, "help", false, "Show help")
	global.BoolVar(&help, "h", false, "Show help")
	global.Usage = func() { fmt.Fprintln(stderr, usageRoot()) }

	if err := global.Parse(argv[1:]); err != nil {
		return 2
	}
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
			opts.Format = "table"
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
		fmt.Fprintln(stderr, "error:", err)
		return 2
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
	default:
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
  --format table|plain|json   Output format (default from config or table)
  --json                Output JSON (alias for --format json)
  --plain               Output plain text (alias for --format plain)
  --ndjson              Output NDJSON (alias for --format ndjson)
  --jsonl               Output NDJSON (alias for --format ndjson)
  --timeout 15s         HTTP timeout
  --api-base <url>      API base URL (default https://www.instapaper.com)
  --debug               Debug output
  --quiet               Less output
  -h, --help            Show help
  --version             Show version

Commands:
  help [command]
  version
  config path|show
  auth login|status|logout
  add <url|-> [--folder <id|"Title">] [--title ...] [--tags "a,b"]
  list [--folder unread|starred|archive|<id>|"Title"] [--limit N] [--tag name] [--have ...] [--highlights ...] [--output <file>]
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
	default:
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
		fmt.Fprintln(stderr, "usage: ip config path|show")
		return 2
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
		if opts != nil && strings.EqualFold(opts.Format, "json") {
			if err := output.WriteJSON(stdout, cfg); err != nil {
				return printError(stderr, err)
			}
			return 0
		}
		if err := printConfig(stdout, cfg); err != nil {
			return printError(stderr, err)
		}
		return 0
	default:
		fmt.Fprintln(stderr, "usage: ip config path|show")
		return 2
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
	if opts.Debug {
		client.EnableDebug(stderr)
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
		fmt.Fprintln(stderr, "usage: ip auth login|status|logout")
		return 2
	}
	switch args[0] {
	case "status":
		if strings.EqualFold(opts.Format, "json") {
			payload := map[string]any{
				"logged_in": cfg.HasAuth(),
			}
			if cfg.HasAuth() {
				payload["user"] = map[string]any{
					"user_id":  cfg.User.UserID,
					"username": cfg.User.Username,
				}
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
		fmt.Fprintln(stdout, "Logged out")
		return 0
	case "login":
		return runAuthLogin(ctx, args[1:], opts, cfg, cfgPath, stdout, stderr)
	default:
		fmt.Fprintln(stderr, "usage: ip auth login|status|logout")
		return 2
	}
}

func runAuthLogin(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, cfgPath string, stdout, stderr io.Writer) int {
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
		fmt.Fprintln(stderr, "error: missing consumer key/secret (set env INSTAPAPER_CONSUMER_KEY/INSTAPAPER_CONSUMER_SECRET or pass flags)")
		return 1
	}

	interactive := isTTY(os.Stdin)
	if username == "" {
		if noInput || !interactive {
			fmt.Fprintln(stderr, "error: missing --username (stdin is not a TTY)")
			return 2
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
			fmt.Fprintln(stderr, "error: --password-stdin requires piped input (stdin is a TTY)")
			return 2
		}
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return printError(stderr, err)
		}
		password = strings.TrimSpace(string(b))
	} else {
		if noInput || !interactive {
			fmt.Fprintln(stderr, "error: missing password; use --password-stdin or run interactively")
			return 2
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
	if opts.Debug {
		client.EnableDebug(stderr)
	}
	ok, sk, err := client.XAuthAccessToken(ctx, username, password)
	// Discard password ASAP.
	password = ""
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
	if opts.Debug {
		client2.EnableDebug(stderr)
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
	fmt.Fprintf(stdout, "Logged in as %s (user_id=%d)\n", cfg.User.Username, cfg.User.UserID)
	return 0
}

// --- bookmarks ---
func runAdd(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageAdd(), fs)
		return 0
	}
	remaining := fs.Args()
	if len(remaining) != 1 {
		fmt.Fprintln(stderr, "usage: ip add <url|-> [flags]")
		return 2
	}
	urlArg := remaining[0]

	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}

	folderID, err := resolveUserFolderID(ctx, client, folder)
	if err != nil {
		return printError(stderr, err)
	}

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
				fmt.Fprintf(stderr, "error adding %s: %v\n", u, err)
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
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var folder string
	var limit int
	var tag string
	var have string
	var highlights string
	var outputPath string
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&folder, "folder", "unread", "Folder: unread|starred|archive|<id>|\"Title\"")
	fs.IntVar(&limit, "limit", cfg.Defaults.ListLimit, "Limit (0 = no limit, max 500)")
	fs.StringVar(&tag, "tag", "", "Tag name (when provided, folder is ignored)")
	fs.StringVar(&have, "have", "", "Comma-separated IDs to exclude (id:progress:timestamp)")
	fs.StringVar(&highlights, "highlights", "", "Comma-separated bookmark IDs for highlights")
	fs.StringVar(&outputPath, "output", "", "Write output to file ('-' for stdout)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageList(), fs)
		return 0
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

	resp, err := client.ListBookmarks(ctx, instapaper.ListBookmarksOptions{
		Limit:      limit,
		FolderID:   folderID,
		Tag:        tag,
		Have:       have,
		Highlights: highlights,
	})
	if err != nil {
		return printError(stderr, err)
	}
	out, closeFn, err := openOutputWriter(outputPath, stdout)
	if err != nil {
		return printError(stderr, err)
	}
	if closeFn != nil {
		defer closeFn()
	}
	if err := output.PrintBookmarks(out, opts.Format, resp.Bookmarks); err != nil {
		return printError(stderr, err)
	}
	if outputPath != "" && outputPath != "-" && !opts.Quiet {
		fmt.Fprintln(stdout, outputPath)
	}
	return 0
}

func runProgress(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
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
		fmt.Fprintln(stderr, "usage: ip progress <bookmark_id> --progress <0..1> --timestamp <unix>")
		return 2
	}
	if progress < 0 || progress > 1 {
		fmt.Fprintln(stderr, "error: --progress must be between 0 and 1")
		return 2
	}
	if timestamp <= 0 {
		fmt.Fprintln(stderr, "error: --timestamp is required (unix seconds)")
		return 2
	}
	id, err := parseInt64(rest[0])
	if err != nil {
		return printError(stderr, err)
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
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageBookmarkMutation(cmd))
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintf(stderr, "usage: ip %s <bookmark_id>\n", cmd)
		return 2
	}
	id, err := parseInt64(args[0])
	if err != nil {
		return printError(stderr, err)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
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
		return printError(stderr, err)
	}
	if opts.Quiet {
		fmt.Fprintf(stdout, "%d\n", int64(bm.BookmarkID))
		return 0
	}
	fmt.Fprintf(stdout, "OK %s %d\n", cmd, int64(bm.BookmarkID))
	return 0
}

func runMove(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
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
		fmt.Fprintln(stderr, "usage: ip move <bookmark_id> --folder <folder_id|\"Title\">")
		return 2
	}
	id, err := parseInt64(remaining[0])
	if err != nil {
		return printError(stderr, err)
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
		fmt.Fprintln(stderr, "error: --folder must be a user folder")
		return 2
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
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var yes bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.BoolVar(&yes, "yes-really-delete", false, "Confirm permanent deletion")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageDelete(), fs)
		return 0
	}
	remaining := fs.Args()
	if len(remaining) != 1 {
		fmt.Fprintln(stderr, "usage: ip delete <bookmark_id> --yes-really-delete")
		return 2
	}
	if !yes {
		fmt.Fprintln(stderr, "refusing: permanent delete requires --yes-really-delete")
		return 2
	}
	id, err := parseInt64(remaining[0])
	if err != nil {
		return printError(stderr, err)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
	if err := client.DeleteBookmark(ctx, id); err != nil {
		return printError(stderr, err)
	}
	if !opts.Quiet {
		fmt.Fprintf(stdout, "Deleted %d\n", id)
	}
	return 0
}

func runText(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("text", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var help bool
	var outPath string
	var openIt bool
	fs.BoolVar(&help, "help", false, "Show help")
	fs.BoolVar(&help, "h", false, "Show help")
	fs.StringVar(&outPath, "out", "", "Write HTML to file")
	fs.BoolVar(&openIt, "open", false, "Open the output file in default browser")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if help {
		printFlagUsage(stdout, usageText(), fs)
		return 0
	}
	remaining := fs.Args()
	if len(remaining) != 1 {
		fmt.Fprintln(stderr, "usage: ip text <bookmark_id> [--out file] [--open]")
		return 2
	}
	id, err := parseInt64(remaining[0])
	if err != nil {
		return printError(stderr, err)
	}
	client, _, _, err := requireClient(opts, cfg, true, stderr)
	if err != nil {
		return printError(stderr, err)
	}
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
		fmt.Fprintln(stderr, "usage: ip folders list|add|delete|order")
		return 2
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
			fmt.Fprintln(stderr, "usage: ip folders add \"Title\"")
			return 2
		}
		f, err := client.AddFolder(ctx, subArgs[0])
		if err != nil {
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
		var help bool
		var yes bool
		fs.BoolVar(&help, "help", false, "Show help")
		fs.BoolVar(&help, "h", false, "Show help")
		fs.BoolVar(&yes, "yes", false, "Confirm delete")
		if err := fs.Parse(subArgs); err != nil {
			return 2
		}
		if help {
			printFlagUsage(stdout, usageFoldersDelete(), fs)
			return 0
		}
		rest := fs.Args()
		if len(rest) != 1 {
			fmt.Fprintln(stderr, "usage: ip folders delete <folder_id|\"Title\"> --yes")
			return 2
		}
		if !yes {
			fmt.Fprintln(stderr, "refusing: folder delete requires --yes")
			return 2
		}
		folderIDStr, err := resolveUserFolderID(ctx, client, rest[0])
		if err != nil {
			return printError(stderr, err)
		}
		if folderIDStr == "" {
			fmt.Fprintln(stderr, "error: must specify a user folder")
			return 2
		}
		id, err := parseInt64(folderIDStr)
		if err != nil {
			return printError(stderr, err)
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
			fmt.Fprintln(stderr, "usage: ip folders order \"100:1,200:2,300:3\"")
			return 2
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
		fmt.Fprintln(stderr, "usage: ip folders list|add|delete|order")
		return 2
	}
}

// --- highlights ---
func runHighlights(ctx context.Context, args []string, opts *GlobalOptions, cfg *config.Config, stdout, stderr io.Writer) int {
	if hasHelpFlag(args) {
		fmt.Fprintln(stdout, usageHighlights())
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: ip highlights list|add|delete")
		return 2
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
			fmt.Fprintln(stderr, "usage: ip highlights list <bookmark_id>")
			return 2
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
			fmt.Fprintln(stderr, "usage: ip highlights add <bookmark_id> --text \"...\" [--position 0]")
			return 2
		}
		bid, err := parseInt64(rest[0])
		if err != nil {
			return printError(stderr, err)
		}
		h, err := client.CreateHighlight(ctx, bid, text, position)
		if err != nil {
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
			fmt.Fprintln(stderr, "usage: ip highlights delete <highlight_id>")
			return 2
		}
		hid, err := parseInt64(subArgs[0])
		if err != nil {
			return printError(stderr, err)
		}
		if err := client.DeleteHighlight(ctx, hid); err != nil {
			return printError(stderr, err)
		}
		if !opts.Quiet {
			fmt.Fprintf(stdout, "Deleted highlight %d\n", hid)
		}
		return 0
	default:
		fmt.Fprintln(stderr, "usage: ip highlights list|add|delete")
		return 2
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
		fmt.Fprintln(stderr, "error:", apiErr.Error())
		if hint := apiErrorHint(apiErr.Code); hint != "" {
			fmt.Fprintln(stderr, "hint:", hint)
		}
		return exitCodeForError(apiErr)
	}
	fmt.Fprintln(stderr, "error:", err)
	return exitCodeForError(err)
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

func usageConfig() string {
	return "Usage:\n  ip config path|show\n"
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
	return "Usage:\n  ip list [flags]\n"
}

func usageBookmarkMutation(cmd string) string {
	return fmt.Sprintf("Usage:\n  ip %s <bookmark_id>\n", cmd)
}

func usageMove() string {
	return "Usage:\n  ip move <bookmark_id> --folder <folder_id|\"Title\">\n"
}

func usageDelete() string {
	return "Usage:\n  ip delete <bookmark_id> --yes-really-delete\n"
}

func usageProgress() string {
	return "Usage:\n  ip progress <bookmark_id> --progress <0..1> --timestamp <unix>\n"
}

func usageText() string {
	return "Usage:\n  ip text <bookmark_id> [--out <file>] [--open]\n"
}

func usageFolders() string {
	return "Usage:\n  ip folders list|add|delete|order\n"
}

func usageFoldersAdd() string {
	return "Usage:\n  ip folders add \"Title\"\n"
}

func usageFoldersDelete() string {
	return "Usage:\n  ip folders delete <folder_id|\"Title\"> --yes\n"
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

func usageAgent() string {
	return `AI agent tips:
  - Use --json for single objects and --ndjson for streams.
  - Prefer --plain only for line-oriented, human-friendly output.
  - Rely on exit codes and error hints on stderr for control flow.
  - For deterministic output, avoid table mode.
Examples:
  ip --json auth status
  ip list --ndjson --limit 0
  ip list --plain --output bookmarks.txt
`
}
