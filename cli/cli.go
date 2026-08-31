package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"text/tabwriter"

	"pastry/config"
	"pastry/pastebin"
)

// Version is injected via -ldflags in release builds.
var Version = "dev"

const devKeyQuestion = "Pastebin API dev key (log in at https://pastebin.com/doc_api#1 to find yours): "

// pasteClient is the subset of *pastebin.Client used by the CLI
type pasteClient interface {
	Create(opts pastebin.CreateOptions) (string, error)
	ReadPublic(key string) ([]byte, error)
	ReadPrivate(key string) ([]byte, error)
	Delete(key string) error
	List(limit int) ([]pastebin.Paste, error)
	Login(username, password string) (string, error)
	UserDetails() (pastebin.User, error)
}

type CLI struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	cfg            *config.Config
	newClient      func(devKey, userKey string) pasteClient
	prompt         func(question string) (string, error)
	promptPassword func(question string) (string, error)
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cli := &CLI{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
	return cli.run(args)
}

func (cli *CLI) setDefaults() {
	if cli.newClient == nil {
		cli.newClient = func(devKey, userKey string) pasteClient {
			return pastebin.New(devKey, userKey)
		}
	}
	if cli.prompt == nil {
		cli.prompt = config.Prompt
	}
	if cli.promptPassword == nil {
		cli.promptPassword = config.PromptPassword
	}
}

func (cli *CLI) run(args []string) int {
	cli.setDefaults()

	if len(args) == 0 {
		if isPiped(cli.stdin) {
			return cli.cmdCreate(args)
		}
		cli.printHelp()
		return 0
	}

	switch args[0] {
	case "help", "--help", "-h":
		cli.printHelp()
		return 0
	case "--version", "-v", "version":
		fmt.Fprintf(cli.stdout, "pastry %s\n", Version)
		return 0
	case "list":
		return cli.cmdList(args[1:])
	case "delete":
		return cli.cmdDelete(args[1:])
	case "whoami":
		return cli.cmdWhoami(args[1:])
	case "login":
		return cli.cmdLogin(args[1:])
	case "logout":
		return cli.cmdLogout(args[1:])
	}

	if !strings.HasPrefix(args[0], "-") {
		return cli.cmdRead(args)
	}

	return cli.cmdCreate(args)
}

// fail prints a "pastry: ..." message to stderr and returns the exit code.
func (cli *CLI) fail(code int, format string, a ...any) int {
	fmt.Fprintf(cli.stderr, "pastry: "+format+"\n", a...)
	return code
}

func (cli *CLI) loadConfig() (*config.Config, error) {
	if cli.cfg != nil {
		return cli.cfg, nil
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	cli.cfg = cfg
	return cfg, nil
}

func (cli *CLI) client(cfg *config.Config) pasteClient {
	return cli.newClient(cfg.DevKey(), cfg.UserKey())
}

// ensureDevKey prompts for and stores the API dev key when missing, returning
// whether a key was newly entered and an exit code (0 on success).
func (cli *CLI) ensureDevKey(cfg *config.Config) (bool, int) {
	if cfg.HasDevKey() {
		return false, 0
	}
	key, err := cli.prompt(devKeyQuestion)
	if err != nil {
		return false, cli.fail(1, "no api dev key configured: %v\n      add 'api_dev_key' to ~/.config/pastry/config.json", err)
	}
	if key == "" {
		return false, cli.fail(1, "empty api dev key")
	}
	cfg.APIDevKey = key
	return true, 0
}

// requireUser loads the config and requires a stored user key, returning the
// config and an exit code (0 on success).
func (cli *CLI) requireUser(cmd string) (*config.Config, int) {
	cfg, err := cli.loadConfig()
	if err != nil {
		return nil, cli.fail(1, "%v", err)
	}
	if !cfg.HasUserKey() {
		return nil, cli.fail(1, "'%s' requires login. Run 'pastry login' first.", cmd)
	}
	return cfg, 0
}

func (cli *CLI) cmdCreate(args []string) int {
	var opts struct {
		title   string
		format  string
		expire  string
		private bool
		guest   bool
	}

	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(cli.stderr)
	fs.StringVar(&opts.title, "title", "", "paste title")
	fs.StringVar(&opts.title, "t", "", "paste title (shorthand)")
	fs.StringVar(&opts.format, "format", "", "syntax highlighting value (e.g. go, python)")
	fs.StringVar(&opts.format, "f", "", "syntax highlighting value (shorthand)")
	fs.StringVar(&opts.expire, "expire", "N", "expiration: N, 10M, 1H, 1D, 1W, 2W, 1M, 6M, 1Y")
	fs.StringVar(&opts.expire, "e", "N", "expiration (shorthand)")
	fs.BoolVar(&opts.private, "private", false, "make the paste private (requires login)")
	fs.BoolVar(&opts.private, "p", false, "make the paste private (shorthand)")
	fs.BoolVar(&opts.guest, "guest", false, "force a guest paste (send no user key, even if logged in)")
	fs.BoolVar(&opts.guest, "g", false, "force a guest paste (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if opts.guest && opts.private {
		return cli.fail(2, "--guest cannot be combined with --private")
	}
	if !validExpires[opts.expire] {
		return cli.fail(2, "invalid --expire %q (valid: N, 10M, 1H, 1D, 1W, 2W, 1M, 6M, 1Y)", opts.expire)
	}

	content, err := io.ReadAll(cli.stdin)
	if err != nil {
		return cli.fail(1, "read stdin: %v", err)
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return cli.fail(1, "no content on stdin (e.g. <command> | pastry)")
	}

	cfg, err := cli.loadConfig()
	if err != nil {
		return cli.fail(1, "%v", err)
	}
	changed, code := cli.ensureDevKey(cfg)
	if code != 0 {
		return code
	}
	if changed {
		if err := config.Save(cfg); err != nil {
			return cli.fail(1, "save config: %v", err)
		}
	}

	privateVal := "1"
	if opts.private {
		privateVal = "2"
	}
	if opts.private && !cfg.HasUserKey() {
		return cli.fail(1, "--private requires login. Run 'pastry login' first.")
	}

	userKey := cfg.UserKey()
	if opts.guest {
		userKey = ""
	}
	link, err := cli.newClient(cfg.DevKey(), userKey).Create(pastebin.CreateOptions{
		Title:   opts.title,
		Format:  opts.format,
		Expire:  opts.expire,
		Private: privateVal,
		Code:    string(content),
	})
	if err != nil {
		return cli.fail(1, "%v", err)
	}
	fmt.Fprintln(cli.stdout, link)
	return 0
}

func (cli *CLI) cmdRead(args []string) int {
	if len(args) < 1 {
		return cli.fail(2, "usage: pastry read <paste-url-or-key>")
	}
	key, err := parseKey(args[0])
	if err != nil {
		return cli.fail(2, "%v", err)
	}

	cfg, err := cli.loadConfig()
	if err != nil {
		cfg = &config.Config{}
	}
	c := cli.client(cfg)

	body, err := c.ReadPublic(key)
	if err != nil {
		// Try reading as private paste if not found
		if errors.Is(err, pastebin.ErrNotFound) && cfg.HasUserKey() {
			body, err = c.ReadPrivate(key)
		}
		if err != nil {
			return cli.fail(1, "%v%s", err, userKeyHint(err))
		}
	}
	if _, err := cli.stdout.Write(body); err != nil {
		return cli.fail(1, "%v", err)
	}
	return 0
}

func (cli *CLI) cmdDelete(args []string) int {
	if len(args) < 1 {
		return cli.fail(2, "usage: pastry delete <paste-url-or-key>")
	}
	key, err := parseKey(args[0])
	if err != nil {
		return cli.fail(2, "%v", err)
	}

	cfg, code := cli.requireUser("delete")
	if code != 0 {
		return code
	}
	if err := cli.client(cfg).Delete(key); err != nil {
		return cli.fail(1, "%v%s", err, userKeyHint(err))
	}
	fmt.Fprintf(cli.stdout, "Paste %s removed\n", key)
	return 0
}

func (cli *CLI) cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(cli.stderr)
	var limit int
	fs.IntVar(&limit, "limit", 50, "max pastes to list (1-1000)")
	fs.IntVar(&limit, "l", 50, "max pastes to list (shorthand)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, code := cli.requireUser("list")
	if code != 0 {
		return code
	}

	pastes, err := cli.client(cfg).List(limit)
	if err != nil {
		return cli.fail(1, "%v%s", err, userKeyHint(err))
	}
	if len(pastes) == 0 {
		fmt.Fprintln(cli.stdout, "No pastes found.")
		return 0
	}

	w := tabwriter.NewWriter(cli.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tTITLE\tDATE\tURL")
	for _, p := range pastes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Key, p.Title, p.Date, p.URL)
	}
	if err := w.Flush(); err != nil {
		return cli.fail(1, "%v", err)
	}
	return 0
}

func (cli *CLI) cmdWhoami(args []string) int {
	cfg, code := cli.requireUser("whoami")
	if code != 0 {
		return code
	}

	u, err := cli.client(cfg).UserDetails()
	if err != nil {
		return cli.fail(1, "%v%s", err, userKeyHint(err))
	}

	fmt.Fprintf(cli.stdout, "username:     %s\n", u.Name)
	fmt.Fprintf(cli.stdout, "format:       %s\n", u.FormatShort)
	fmt.Fprintf(cli.stdout, "expiration:   %s\n", u.Expiration)
	fmt.Fprintf(cli.stdout, "private:      %s\n", u.Private)
	fmt.Fprintf(cli.stdout, "website:      %s\n", u.Website)
	fmt.Fprintf(cli.stdout, "location:     %s\n", u.Location)
	fmt.Fprintf(cli.stdout, "account type: %s\n", u.AccountType)
	return 0
}

func (cli *CLI) cmdLogin(args []string) int {
	cfg, err := cli.loadConfig()
	if err != nil {
		return cli.fail(1, "%v", err)
	}
	_, code := cli.ensureDevKey(cfg)
	if code != 0 {
		return code
	}

	username, err := cli.prompt("Pastebin username: ")
	if err != nil {
		return cli.fail(1, "%v", err)
	}
	if username == "" {
		return cli.fail(1, "username is required")
	}

	password, err := cli.promptPassword("Pastebin password: ")
	if err != nil {
		return cli.fail(1, "%v", err)
	}

	userKey, err := cli.newClient(cfg.DevKey(), "").Login(username, password)
	if err != nil {
		return cli.fail(1, "%v", err)
	}

	cfg.APIUserKey = userKey
	if err := config.Save(cfg); err != nil {
		return cli.fail(1, "save config: %v", err)
	}
	fmt.Fprintln(cli.stdout, "Logged in. api_user_key stored.")
	return 0
}

func (cli *CLI) cmdLogout(args []string) int {
	cfg, err := cli.loadConfig()
	if err != nil {
		return cli.fail(1, "%v", err)
	}
	if !cfg.HasUserKey() {
		fmt.Fprintln(cli.stdout, "Not logged in.")
		return 0
	}
	cfg.APIUserKey = ""
	if err := config.Save(cfg); err != nil {
		return cli.fail(1, "save config: %v", err)
	}
	fmt.Fprintln(cli.stdout, "Logged out. api_user_key removed.")
	return 0
}

func (cli *CLI) printHelp() {
	w := cli.stdout
	fmt.Fprintln(w, "pastry - a pastebin.com CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  <command> | pastry                 create a paste from stdin")
	fmt.Fprintln(w, "      -t, --title T       paste title")
	fmt.Fprintln(w, "      -f, --format F      syntax highlighting (e.g. go, python)")
	fmt.Fprintln(w, "      -e, --expire E      expiration: N, 10M, 1H, 1D, 1W, 2W, 1M, 6M, 1Y")
	fmt.Fprintln(w, "      -g, --guest         force a guest paste (no user key)")
	fmt.Fprintln(w, "      -p, --private       private paste (requires login)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  pastry <url-or-key>                read a paste")
	fmt.Fprintln(w, "  pastry list [-l|--limit N]         list your pastes (login required)")
	fmt.Fprintln(w, "  pastry delete <url-or-key>         delete a paste (login required)")
	fmt.Fprintln(w, "  pastry whoami                      show account info (login required)")
	fmt.Fprintln(w, "  pastry login                       obtain and store your api_user_key")
	fmt.Fprintln(w, "  pastry logout                      remove your stored api_user_key")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Configuration: ~/.config/pastry/config.json")
}

func userKeyHint(err error) string {
	var ae *pastebin.APIError
	if errors.As(err, &ae) && strings.Contains(ae.Message, "api_user_key") {
		return " (your api_user_key may be invalid or expired - run 'pastry login')"
	}
	return ""
}

// parseKey extracts a paste key from a bare key or a pastebin.com URL.
func parseKey(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("empty paste URL or key")
	}
	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return "", fmt.Errorf("invalid URL %q: %v", s, err)
		}
		s = u.Path
	}
	s = strings.Trim(s, "/")
	key := path.Base(s)
	if key == "." || key == "/" {
		return "", fmt.Errorf("could not extract a paste key from %q", s)
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return "", fmt.Errorf("invalid paste key %q", key)
		}
	}
	return key, nil
}

// isPiped reports whether stdin is a pipe rather than a terminal.
func isPiped(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return true
	}
	fi, err := f.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

var validExpires = map[string]bool{
	"N": true, "10M": true, "1H": true, "1D": true,
	"1W": true, "2W": true, "1M": true, "6M": true, "1Y": true,
}
