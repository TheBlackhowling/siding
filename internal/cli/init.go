package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBlackhowling/siding/internal/advise"
	"github.com/TheBlackhowling/siding/internal/compose"
	"github.com/TheBlackhowling/siding/internal/contract"
	"github.com/TheBlackhowling/siding/internal/gitinfo"
	"github.com/TheBlackhowling/siding/internal/hostport"
	"github.com/TheBlackhowling/siding/internal/paths"
	"github.com/TheBlackhowling/siding/internal/proxy"
	"github.com/TheBlackhowling/siding/internal/state"
)

var stdinReader = bufio.NewReader(os.Stdin)

func initCmd() *cobra.Command {
	var project, prefix string
	var composeFiles, ports, modeNames, httpPrefixes, composeEnvFiles []string
	var primaryBranch string
	var force, preferProxy, rewriteCompose, printHosts, dryRun bool
	var hostPrefixes, entries []string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a recipe, or sync an existing org recipe onto this machine",
		Long: `Walk through ports vs the shared Caddy proxy, optionally copy compose into .siding/compose/, and write .siding/config.yml.

If this repo already has a committed .siding/config.yml (org recipe), init
does not overwrite it — it runs siding sync to map that recipe onto local
state.json and .siding/env. Pass --force to re-run the wizard.

The wizard never edits the OS hosts file. *.localhost must not be added to
it — browsers and the OS already resolve those names.

Interactive on a TTY. The wizard lists compose files at the repo root; pick
numbers (1,3) or all, then a suffix per stack (test, prod, …), then repo env
files for each mode (that mode's docker compose --env-file list only; not
.siding/env), then your public host prefix (example →
app/www/api.example.{stack}.{mode}.localhost), then HTTP entry points
(web=app, api=api) that become clickable board links. Override files attach
to the primary compose file. Non-interactive:

  siding init --project hello --compose docker-compose.yml --compose docker-compose.prod-db.yml --mode-name docker-compose.yml=test --mode-name docker-compose.prod-db.yml=prod --compose-env-file prod=docker/.env.prod-db`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			start, err := startPathFlag(cmd)
			if err != nil {
				return err
			}
			repo := start
			if root, err := contract.FindRepoRoot(start); err == nil {
				repo = root
			}
			if contract.HasContract(repo) && !force {
				if rewriteCompose {
					warnf("note: org recipe exists — skipping compose copy (pass --force to re-run the wizard)\n")
				}
				return runSync(cmd, "", "test", syncOpts{
					printHosts: printHosts,
					dryRun:     dryRun,
					fromInit:   true,
				})
			}

			opts := initOpts{
				project:        project,
				prefix:         prefix,
				composeFiles:   composeFiles,
				ports:          ports,
				modeNames:        modeNames,
				composeEnvFiles:  composeEnvFiles,
				preferProxy:    preferProxy,
				rewriteCompose: rewriteCompose,
				printHosts:     printHosts,
				dryRun:         dryRun,
				askProxy:       !cmd.Flags().Changed("prefer-proxy"),
				askRewrite:     !cmd.Flags().Changed("rewrite-compose"),
				httpPrefixes:   append(append([]string{}, hostPrefixes...), httpPrefixes...),
				entries:        entries,
				askPrefix:      !cmd.Flags().Changed("folder-prefix"),
				askHTTPPrefix:  !cmd.Flags().Changed("http-prefix") && !cmd.Flags().Changed("host-prefix"),
				askEntries:     !cmd.Flags().Changed("entry"),
				askCompose:     !cmd.Flags().Changed("compose"),
				askModes:       !cmd.Flags().Changed("mode-name"),
				askComposeEnv:     !cmd.Flags().Changed("compose-env-file"),
				primaryBranch:     primaryBranch,
				askPrimaryBranch:  !cmd.Flags().Changed("primary-branch"),
			}
			c, err := runInit(repo, opts)
			if err != nil {
				return err
			}
			if dryRun {
				println(yellow("dry run — config and compose not written"))
				return nil
			}
			path, err := contract.Write(repo, c)
			if err != nil {
				return err
			}
			printf("%s %s\n", green("wrote"), path)
			println()
			return runSync(cmd, "", "test", syncOpts{})
		},
	}
	addPathFlag(cmd)
	cmd.Flags().StringVar(&project, "project", "", "Product id shared by every clone (default: folder prefix without the trailing hyphen, else folder name)")
	cmd.Flags().StringVar(&prefix, "folder-prefix", "", "Strip this prefix from folder names to get the stack (e.g. acme- → genie3)")
	cmd.Flags().StringArrayVar(&httpPrefixes, "http-prefix", nil, "Product host prefix without app. (example → app/www/api.example.{stack}.{mode}.localhost). none = {stack}.{project}.{mode}")
	cmd.Flags().StringArrayVar(&hostPrefixes, "host-prefix", nil, "Alias for --http-prefix")
	cmd.Flags().StringArrayVar(&entries, "entry", nil, "HTTP entry point KEY=LABEL (repeatable). web=app api=api. Comma extras: web=app,www")
	cmd.Flags().StringArrayVar(&composeFiles, "compose", nil, "Root compose file to process (repeatable). Default: wizard list, or all when non-interactive")
	cmd.Flags().StringArrayVar(&modeNames, "mode-name", nil, "Suffix for a compose file FILE=SUFFIX (repeatable), e.g. docker-compose.prod-db.yml=prod")
	cmd.Flags().StringArrayVar(&composeEnvFiles, "compose-env-file", nil, "Repo --env-file for one mode only (repeatable MODE=PATH). Not shared across modes. Example: prod=docker/.env.prod-db")
	cmd.Flags().StringVar(&primaryBranch, "primary-branch", "", "Primary git branch for worktree start points (default: origin/HEAD or main)")
	cmd.Flags().StringArrayVar(&ports, "port", nil, "Default port KEY=NUMBER (repeatable)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing contract")
	cmd.Flags().BoolVar(&preferProxy, "prefer-proxy", true, "Use Host URLs via siding proxy (Caddy on host :80)")
	cmd.Flags().BoolVar(&rewriteCompose, "rewrite-compose", false, "Copy root compose files to .siding/compose/ and adapt the copies (originals stay for non-siding users)")
	cmd.Flags().BoolVar(&printHosts, "print-hosts", false, "Remind that *.localhost must not be added to the hosts file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show the recipe and compose-copy plan without writing")
	return cmd
}

type initOpts struct {
	project, prefix                                                                  string
	composeFiles, ports, modeNames, composeEnvFiles                                  []string
	httpPrefixes                                                                     []string
	entries                                                                          []string
	preferProxy                                                                      bool
	rewriteCompose                                                                   bool
	printHosts                                                                       bool
	dryRun                                                                           bool
	askProxy, askRewrite, askPrefix, askHTTPPrefix, askEntries, askCompose, askModes bool
	askComposeEnv                                                                    bool
	primaryBranch                                                                    string
	askPrimaryBranch                                                                 bool
}

func runInit(repo string, opts initOpts) (*contract.Contract, error) {
	tty := isTTY()
	base := paths.Basename(repo)
	if isNone(opts.prefix) {
		opts.prefix = ""
	}
	if opts.askPrefix && opts.prefix == "" {
		if guess := guessFolderPrefix(base); guess != "" {
			if tty {
				raw := prompt("Folder prefix to strip (shared by clones; none = off)", guess)
				if !isNone(raw) {
					opts.prefix = strings.TrimSpace(raw)
				}
			} else {
				// Same default as the TTY prompt so scripted init cannot
				// bake the clone folder into project (acme-genie3).
				opts.prefix = guess
			}
		}
	}
	project := opts.project
	if project == "" {
		project = contract.DefaultProject(base, opts.prefix)
		if tty {
			project = prompt("Project name (product id, shared by every clone)", project)
		}
	}
	composeFiles, err := pickComposeFiles(repo, opts, tty)
	if err != nil {
		return nil, err
	}
	groups := compose.GroupFiles(composeFiles)
	if len(groups) == 0 {
		return nil, fmt.Errorf("no compose files to process")
	}
	groups, err = pickModeSuffixes(groups, opts, tty)
	if err != nil {
		return nil, err
	}

	flagPorts, flagEnv, err := parsePortFlags(opts.ports)
	if err != nil {
		return nil, err
	}
	scanned := advise.ScanRepo(repo, composeFiles, nil, "")
	guessedPrefix := scanned.HostPrefix

	println()
	heading("Detected compose wiring")
	if !advise.HasProblems(scanned) && len(scanned.Findings) > 0 {
		println(dim("  published ports already use ${ENV} interpolations"))
	}
	for _, f := range scanned.Findings {
		switch f.Kind {
		case "port_ok":
			printf("  %s %s\n", green("ok   "), f.Detail)
		case "network_ok", "network_isolated", "host_network":
			// printed under Docker network
		default:
			printf("  %s %s\n", yellow("need "), f.Detail)
		}
	}
	if len(scanned.Network.Services) > 0 {
		println()
		heading("Docker network")
		for _, line := range advise.FormatNetworkStatus(scanned.Network) {
			if scanned.Network.NeedsEnsure {
				printf("  %s %s\n", yellow("need "), line)
			} else {
				printf("  %s %s\n", green("ok   "), line)
			}
		}
	}

	preferProxy := opts.preferProxy
	if tty && opts.askProxy {
		println()
		heading("How should the browser reach the frontend?")
		printf("  %s Shared siding proxy — Caddy on host :80, Host http://<sub>.<host> or http://<host>\n", green("[Y]"))
		println(dim("      Port 80 must be free. NEXT_PUBLIC_* stays portless (not :3000)."))
		printf("  %s Raw published ports — public URLs include the allocated host port (:18000, …).\n", yellow("[n]"))
		println(dim("Browsers resolve *.localhost with no hosts file either way."))
		preferProxy = promptYesNo("Use the shared Caddy proxy for frontend redirects?", true)
	}
	if preferProxy {
		ok, err := confirmProxyPortFree(tty)
		if err != nil {
			return nil, err
		}
		if !ok {
			preferProxy = false
		}
	}

	needsHelp := advise.HasProblems(scanned)
	rewrite := opts.rewriteCompose
	if tty && opts.askRewrite {
		println()
		heading("Siding can copy those compose files into .siding/compose/ and adapt the copies:")
		println("  • originals at the repo root stay as they are (non-siding docker compose still works)")
		println("  • copies get ${WEB_PORT:-…} / ${API_PORT:-…} on published ports")
		println("  • NEXT_PUBLIC_*/VITE_*/DEV_* :port defaults → values from .siding/env")
		println("  • in-container localhost → Compose service DNS (http://backend:8000)")
		println("  • isolated services / network_mode: host → one shared project network")
		println("Healthchecks and GCS/emulator localhost are left alone.")
		println("App code that reads NEXT_PUBLIC_* from env may still need a one-line wrap on the")
		println("original (${NEXT_PUBLIC_ROOT_DOMAIN:-old-default}) so both workflows work.")
		rewrite = promptYesNo("Write siding compose copies under .siding/compose/?", needsHelp)
	}

	fileToMode := map[string]string{}
	for _, g := range groups {
		for _, f := range g.Files {
			fileToMode[f] = g.Mode
		}
	}
	publicByMode := map[string]map[string]string{}
	putPublic := func(file, key, val string) {
		m := fileToMode[compose.RelComposePath(file)]
		if m == "" {
			m = "test"
		}
		if publicByMode[m] == nil {
			publicByMode[m] = map[string]string{}
		}
		publicByMode[m][key] = val
	}
	for _, f := range scanned.Findings {
		if f.Kind != "public_url" {
			continue
		}
		parts := strings.SplitN(f.Detail, ": ", 3)
		if len(parts) >= 2 {
			key := strings.Fields(parts[1])[0]
			putPublic(parts[0], key, advise.SuggestedPublicTemplate(key))
		}
	}

	if rewrite {
		origToSiding := map[string]string{}
		for _, composeFile := range composeFiles {
			src := filepath.Join(repo, composeFile)
			data, readErr := os.ReadFile(src)
			if readErr != nil {
				return nil, readErr
			}
			body := string(data)
			idx := advise.ContainerPortIndex(body)
			next, changes, foundPub := advise.RewriteCompose(body, idx)
			for k, v := range foundPub {
				putPublic(composeFile, k, v)
			}
			destRel := contract.SidingComposeRel(composeFile)
			origToSiding[composeFile] = destRel
			println()
			if len(changes) == 0 {
				printf("%s %s ← %s %s\n", dim("siding compose"), destRel, composeFile, dim("(copy, no edits)"))
			} else {
				printf("%s %s ← %s (%s)\n", dim("siding compose"), destRel, composeFile, yellow(fmt.Sprintf("%d changes", len(changes))))
				for _, ch := range changes {
					printf("  %s\n    %s %s\n    %s %s\n", dim(ch.Reason), red("-"), ch.Before, green("+"), ch.After)
				}
			}
			if !opts.dryRun {
				if _, wrErr := advise.WriteSidingCompose(repo, composeFile, next); wrErr != nil {
					return nil, wrErr
				}
			}
		}
		for i := range groups {
			for j, f := range groups[i].Files {
				if d, ok := origToSiding[f]; ok {
					groups[i].Files[j] = d
				}
			}
		}
	}

	modes := map[string]contract.Mode{}
	var primaryName string
	var primaryPorts map[string]int
	totalPorts := 0
	for i, g := range groups {
		scanFiles := g.Files
		if rewrite && opts.dryRun {
			// Copies are not on disk yet; scan the originals for port ownership.
			scanFiles = make([]string, len(g.Files))
			for j, f := range g.Files {
				scanFiles[j] = filepath.Base(f)
			}
		}
		scan := advise.ScanRepo(repo, scanFiles, nil, "")
		var ports map[string]int
		var env map[string]string
		if i == 0 {
			ports, env = advise.OwnPublished(flagPorts, flagEnv, scan)
		} else {
			ports, env = advise.OwnPublished(nil, nil, scan)
		}
		modeEnv := map[string]string{}
		for k, v := range publicByMode[g.Mode] {
			modeEnv[k] = v
		}
		for _, f := range scan.Findings {
			if f.Kind != "public_url" {
				continue
			}
			parts := strings.SplitN(f.Detail, ": ", 3)
			if len(parts) >= 2 {
				key := strings.Fields(parts[1])[0]
				if modeEnv[key] == "" {
					modeEnv[key] = advise.SuggestedPublicTemplate(key)
				}
			}
		}
		totalPorts += len(ports)
		if i == 0 {
			primaryName = g.Mode
			primaryPorts = ports
		}
		if len(modeEnv) == 0 {
			modeEnv = nil
		}
		if p := scan.HostPrefix; p != "" && strings.Count(p, ".") > strings.Count(guessedPrefix, ".") {
			guessedPrefix = p
		}
		httpHosts := scan.HTTPHosts
		if len(httpHosts) == 0 {
			httpHosts = nil
		}
		modes[g.Mode] = contract.Mode{
			ComposeFiles: g.Files,
			Ports:        ports,
			PortEnv:      env,
			Env:          modeEnv,
			HTTPHosts:    httpHosts,
		}
	}
	if err := attachComposeEnvFiles(repo, modes, opts, tty); err != nil {
		return nil, err
	}
	if primaryName != "" && len(primaryPorts) == 0 && totalPorts == 0 {
		web := 18080
		if tty {
			if raw := prompt("Default web port (host bind when proxy is off)", "18080"); raw != "" {
				n, convErr := strconv.Atoi(raw)
				if convErr != nil {
					return nil, fmt.Errorf("web port: %w", convErr)
				}
				web = n
			}
		}
		m := modes[primaryName]
		m.Ports = map[string]int{"web": web}
		m.PortEnv = map[string]string{"web": "WEB_PORT"}
		modes[primaryName] = m
	}

	primaryBranch, err := pickPrimaryBranch(context.Background(), repo, opts, tty)
	if err != nil {
		return nil, err
	}

	c := &contract.Contract{
		Version:        1,
		Project:        project,
		FolderPrefix:   strings.TrimSpace(opts.prefix),
		PrimaryBranch:  primaryBranch,
		Host:           contract.DefaultHost,
		ComposeProject: contract.DefaultCompose,
		SlotEnvFile:    contract.DefaultSlotEnv,
		Modes:          modes,
	}
	chosen, err := pickHostPrefix(opts, tty, guessedPrefix)
	if err != nil {
		return nil, err
	}
	contract.ApplyHostPrefix(c, chosen)
	pickedEntries, err := pickEntryPoints(opts, tty, c)
	if err != nil {
		return nil, err
	}
	contract.ApplyEntryPoints(c, pickedEntries)

	stack, err := contract.ResolveStackName(c, repo, "")
	if err != nil {
		return nil, err
	}
	defaultMode, err := contract.PickMode(c, "test")
	if err != nil {
		return nil, err
	}
	mode, _ := c.Mode(defaultMode)
	host, err := c.ExpandHost(stack, defaultMode, mode)
	if err != nil {
		return nil, err
	}

	printf("%s %s\n", dim("primary_branch:"), c.PrimaryBranch)

	println()
	heading("modes (each is its own compose project + ports):")
	for _, name := range contract.ModeNames(c) {
		m := c.Modes[name]
		printf("  %s\n", bold(name))
		for _, f := range m.ComposeFiles {
			printf("    %s\n", dim(f))
		}
		for _, f := range m.ComposeEnvFiles {
			line := f.Path
			if f.Optional {
				line += " (optional)"
			}
			printf("    %s %s\n", dim("compose_env_files:"), line)
		}
		if len(m.Ports) == 0 {
			println(dim("    (no published ports)"))
			continue
		}
		keys := make([]string, 0, len(m.Ports))
		for k := range m.Ports {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			env := m.PortEnv[k]
			if env == "" {
				env = advise.EnvNameForKey(k)
			}
			printf("    %s: %d (${%s})\n", k, m.Ports[k], env)
		}
	}
	if preferProxy {
		printf("%s %s — after init, run:  %s\n", dim("proxy: "), green("yes"), cyan("siding proxy up"))
		printf("%s %s\n", dim("board: "), cyan("http://siding.localhost"))
		printed := false
		for _, key := range contract.HTTPEntryKeys {
			if _, ok := mode.Ports[key]; !ok {
				continue
			}
			for _, label := range contract.ResolveEntryPoints(c, mode, mode.Ports)[key] {
				ep := contract.EntryHost(host, label)
				if !printed {
					printf("%s %s  %s\n", dim("urls:  "), cyan("http://"+ep), dim(fmt.Sprintf("(%s · mode %s; Caddy on host :80)", label, defaultMode)))
					printed = true
					continue
				}
				printf("        %s  %s\n", cyan("http://"+ep), dim("("+label+")"))
			}
		}
		if len(c.Modes) > 1 {
			printf("        %s %s\n", dim("other modes:"), cyan("siding up --mode NAME"))
		}
	} else {
		printf("%s %s\n", dim("proxy: "), yellow("no — public URLs will include the allocated host port"))
		if p := mode.Ports["web"]; p > 0 {
			printf("%s %s\n", dim("urls:  "), cyan(fmt.Sprintf("http://%s:%d", host, p)))
		}
	}

	if opts.printHosts {
		println()
		printLocalhostHostsNote()
	}

	println()
	printf("%s  %s\n", bold("next:"), cyan("siding doctor && siding plan"))
	if preferProxy {
		printf("       %s\n", cyan("siding proxy up && siding up"))
	} else {
		printf("       %s\n", cyan("siding up"))
	}
	return c, nil
}

func pickComposeFiles(repo string, opts initOpts, tty bool) ([]string, error) {
	found, err := compose.ListRootComposeFiles(repo)
	if err != nil {
		return nil, err
	}
	if !opts.askCompose && len(opts.composeFiles) > 0 {
		var out []string
		for _, raw := range opts.composeFiles {
			name := compose.RelComposePath(raw)
			path := filepath.Join(repo, name)
			if _, statErr := os.Stat(path); statErr != nil {
				return nil, fmt.Errorf("compose file %s: %w", name, statErr)
			}
			out = append(out, name)
		}
		println()
		heading("Compose files (--compose)")
		for i, f := range out {
			printf("  %d. %s  (guess: %s)\n", i+1, f, composeGuess(f))
		}
		return out, nil
	}

	if len(found) == 0 {
		name := "docker-compose.yml"
		if tty {
			name = prompt("No compose files at repo root. Compose file", name)
		}
		if name == "" {
			return nil, fmt.Errorf("no compose files at %s", repo)
		}
		return []string{compose.RelComposePath(name)}, nil
	}

	println()
	heading("Compose files at repo root")
	println(dim("Siding will scan, own every published port, and can copy the ones you pick"))
	println(dim("into .siding/compose/ (originals stay for non-siding users)."))
	println(dim("Each compose file is its own stack (own ports and compose project) except *override*"))
	println(dim("files, which stay with docker-compose.yml / compose.yaml."))
	for i, f := range found {
		printf("  %d. %s  (guess: %s)\n", i+1, f, composeGuess(f))
	}
	if !tty || !opts.askCompose {
		println("  using all (non-interactive; pass --compose FILE to limit)")
		return found, nil
	}
	raw := prompt("Process which files? (comma-separated numbers or all)", "all")
	selected, err := compose.ParseComposeSelection(raw, found)
	if err != nil {
		return nil, err
	}
	println("  selected:")
	for _, f := range selected {
		printf("    %s\n", f)
	}
	return selected, nil
}

func pickModeSuffixes(groups []compose.FileGroup, opts initOpts, tty bool) ([]compose.FileGroup, error) {
	byFile, err := parseModeNameFlags(opts.modeNames)
	if err != nil {
		return nil, err
	}
	if tty && opts.askModes {
		println()
		heading("Mode suffix for each compose stack")
		println(dim("Used in hosts (stack.project.SUFFIX.localhost) and:  siding up --mode SUFFIX"))
		println(dim("siding up with no --mode uses \"test\" when that suffix exists."))
		used := map[string]struct{}{}
		prompted := map[string]string{}
		for _, g := range groups {
			def := g.Mode
			for _, f := range g.Files {
				if n := byFile[f]; n != "" {
					def = n
					break
				}
			}
			label := strings.Join(g.Files, " + ")
			for {
				printf("  %s\n", label)
				printf("  current guess: %s\n", def)
				raw := compose.SanitizeModeName(prompt("  suffix", def))
				if raw == "" {
					raw = def
				}
				if _, taken := used[raw]; taken {
					printf("  %s\n", yellow(fmt.Sprintf("%q is already used — pick another suffix", raw)))
					continue
				}
				used[raw] = struct{}{}
				prompted[g.Files[0]] = raw
				break
			}
		}
		return compose.ApplyModeNames(groups, prompted), nil
	}
	if len(byFile) > 0 {
		groups = compose.ApplyModeNames(groups, byFile)
	}
	println()
	heading("Mode suffixes")
	for _, g := range groups {
		printf("  %s → %s\n", strings.Join(g.Files, " + "), g.Mode)
	}
	return groups, nil
}

func pickPrimaryBranch(ctx context.Context, repo string, opts initOpts, tty bool) (string, error) {
	if b := strings.TrimSpace(opts.primaryBranch); b != "" {
		return b, nil
	}
	branch := gitinfo.DefaultRemoteBranch(ctx, repo)
	if branch == "" {
		branch = contract.DefaultPrimaryBranch
	}
	if tty && opts.askPrimaryBranch {
		println()
		heading("Primary git branch")
		println(dim("Default start point for siding worktree add when from is omitted."))
		branch = prompt("Branch name", branch)
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = contract.DefaultPrimaryBranch
	}
	return branch, nil
}

func attachComposeEnvFiles(repo string, modes map[string]contract.Mode, opts initOpts, tty bool) error {
	flagged, err := parseComposeEnvFileFlags(opts.composeEnvFiles)
	if err != nil {
		return err
	}
	if tty && opts.askComposeEnv {
		println()
		heading("Repo env files for each mode")
		println(dim("Each mode has its own list. These become docker compose --env-file"))
		println(dim("on `siding up --mode NAME` so ${VAR} in that stack can interpolate."))
		println(dim("They are not .siding/env or .siding/env.<mode>. A mode only sees"))
		println(dim("files that belong to its compose stack. none = generated slot file only."))
	}
	for _, name := range contract.ModeNames(&contract.Contract{Modes: modes}) {
		m := modes[name]
		all, guessed := compose.ListComposeEnvFiles(repo, name, m.ComposeFiles)
		if tty && opts.askComposeEnv {
			if len(all) == 0 {
				printComposeEnvModeHeader(name, m.ComposeFiles)
				println(dim("    no repo env files for this mode"))
				m.ComposeEnvFiles = nil
				modes[name] = m
				continue
			}
			entries, err := promptComposeEnvFiles(name, m.ComposeFiles, all, guessed)
			if err != nil {
				return err
			}
			m.ComposeEnvFiles = entries
			modes[name] = m
			continue
		}
		m.ComposeEnvFiles = contract.ComposeEnvFilesFromPaths(flagged[name])
		modes[name] = m
		if extra := composeEnvFilesNotChosen(all, m.ComposeEnvFiles.Paths()); len(extra) > 0 {
			warnf("warning: mode %s has its own env files not passed as --compose-env-file: %s — pick them on a TTY or pass --compose-env-file %s=PATH (per mode)\n",
				name, strings.Join(extra, ", "), name)
		}
	}
	return nil
}

func composeEnvFilesNotChosen(detected, chosen []string) []string {
	have := map[string]struct{}{}
	for _, f := range chosen {
		have[f] = struct{}{}
	}
	var extra []string
	for _, f := range detected {
		if _, ok := have[f]; !ok {
			extra = append(extra, f)
		}
	}
	return extra
}

func printComposeEnvModeHeader(mode string, composeFiles []string) {
	println()
	printf("  %s  %s\n", bold("mode "+mode), dim("siding up --mode "+mode))
	if len(composeFiles) > 0 {
		printf("    %s\n", dim(strings.Join(composeFiles, " + ")))
	}
}

func promptComposeEnvFiles(mode string, composeFiles, all, guessed []string) (contract.ComposeEnvFileList, error) {
	printComposeEnvModeHeader(mode, composeFiles)
	guessedSet := map[string]struct{}{}
	for _, g := range guessed {
		guessedSet[g] = struct{}{}
	}
	for i, f := range all {
		mark := ""
		if _, ok := guessedSet[f]; ok {
			mark = "  " + dim("(guess)")
		}
		printf("    %d. %s%s\n", i+1, f, mark)
	}
	def := compose.DefaultEnvFileSelection(all, guessed)
	raw := prompt("  files for this mode only (numbers, none, or all)", def)
	selected, err := compose.ParseEnvFileSelection(raw, all)
	if err != nil {
		return nil, err
	}
	var entries contract.ComposeEnvFileList
	for _, f := range selected {
		optional := promptYesNo(fmt.Sprintf("  skip %s if missing on disk?", f), true)
		entries = append(entries, contract.ComposeEnvFile{Path: f, Optional: optional})
	}
	if len(entries) > 0 {
		printf("    selected for %s:\n", mode)
		for _, e := range entries {
			line := e.Path
			if e.Optional {
				line += dim(" (optional)")
			}
			printf("      %s\n", line)
		}
	}
	return entries, nil
}

func parseComposeEnvFileFlags(flags []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(flags) == 0 {
		return out, nil
	}
	for _, f := range flags {
		mode, path, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf("compose-env-file must be MODE=PATH (got %q)", f)
		}
		mode = compose.SanitizeModeName(mode)
		path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
		if mode == "" || path == "" {
			return nil, fmt.Errorf("compose-env-file must be MODE=PATH (got %q)", f)
		}
		out[mode] = append(out[mode], path)
	}
	return out, nil
}

func parseModeNameFlags(flags []string) (map[string]string, error) {
	if len(flags) == 0 {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	for _, f := range flags {
		file, suffix, ok := strings.Cut(f, "=")
		if !ok {
			return nil, fmt.Errorf("mode-name must be FILE=SUFFIX (got %q)", f)
		}
		file = compose.RelComposePath(file)
		suffix = compose.SanitizeModeName(suffix)
		if file == "" || suffix == "" {
			return nil, fmt.Errorf("mode-name must be FILE=SUFFIX (got %q)", f)
		}
		out[file] = suffix
	}
	return out, nil
}

func composeGuess(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "override") {
		return "override (with primary compose)"
	}
	return compose.ModeNameFromFile(name)
}

func parsePortFlags(flags []string) (map[string]int, map[string]string, error) {
	if len(flags) == 0 {
		return nil, nil, nil
	}
	ports := map[string]int{}
	env := map[string]string{}
	for _, f := range flags {
		key, val, ok := strings.Cut(f, "=")
		if !ok {
			return nil, nil, fmt.Errorf("port must be KEY=NUMBER (got %q)", f)
		}
		n, err := strconv.Atoi(val)
		if err != nil {
			return nil, nil, fmt.Errorf("port %s: %w", key, err)
		}
		ports[key] = n
		env[key] = strings.ToUpper(key) + "_PORT"
	}
	return ports, env, nil
}

// isTTY decides whether the init wizard prompts. Tests set it explicitly.
var isTTY = stdinIsTTY

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func pickHostPrefix(opts initOpts, tty bool, guess string) (string, error) {
	if !opts.askHTTPPrefix {
		if len(opts.httpPrefixes) == 0 {
			return "", nil
		}
		return contract.ParseHostPrefixInput(opts.httpPrefixes[len(opts.httpPrefixes)-1]), nil
	}
	if !tty {
		return guess, nil
	}
	println()
	heading("Public host prefix")
	println(dim("Product name only — do not include app. or www."))
	println(dim("Stack and mode are appended. These subdomains all work:"))
	println()
	printf("  %s  →  %s\n", bold("example"), cyan("http://app.example.my-stack.test.localhost"))
	printf("              %s\n", cyan("http://www.example.my-stack.test.localhost"))
	printf("              %s\n", cyan("http://api.example.my-stack.test.localhost"))
	println()
	printf("  %s     →  %s\n", bold("none"), dim("{stack}.{project}.{mode}.localhost"))
	println()
	println(dim("*.localhost is not added to the hosts file."))
	def := guess
	if def == "" {
		def = "none"
	}
	return contract.ParseHostPrefixInput(prompt("Public host prefix", def)), nil
}

func pickEntryPoints(opts initOpts, tty bool, c *contract.Contract) (map[string][]string, error) {
	keys := httpEntryKeysFromContract(c)
	if len(keys) == 0 {
		return nil, nil
	}
	out := map[string][]string{}
	if !opts.askEntries {
		parsed, err := parseEntryFlags(opts.entries)
		if err != nil {
			return nil, err
		}
		for k, v := range parsed {
			out[k] = v
		}
	} else if tty {
		println()
		heading("HTTP entry points")
		printf("%s %s\n", dim("Subdomains for web containers — clickable on"), cyan("http://siding.localhost."))
		printf("  %s  %s  →  %s\n", dim("web (frontend)"), bold("app"), cyan("http://app.{prefix}.{stack}.{mode}.localhost"))
		printf("  %s   %s  →  %s\n", dim("api (backend)"), bold("api"), cyan("http://api.{prefix}.{stack}.{mode}.localhost"))
		println(dim("Comma-separated extras are fine (app,www). Use web for the apex host."))
		println()
		for _, key := range keys {
			def := strings.Join(defaultEntryLabels(key, c), ",")
			role := "frontend"
			if key == "api" {
				role = "backend"
			}
			raw := prompt(fmt.Sprintf("%s (%s) entry", key, role), def)
			if strings.EqualFold(strings.TrimSpace(raw), "none") {
				out[key] = defaultEntryLabels(key, c)
				continue
			}
			_, labels, err := contract.ParseEntryFlag(key + "=" + raw)
			if err != nil {
				return nil, err
			}
			out[key] = labels
		}
	}
	for _, key := range keys {
		if len(out[key]) == 0 {
			out[key] = defaultEntryLabels(key, c)
		}
	}
	return out, nil
}

func parseEntryFlags(flags []string) (map[string][]string, error) {
	out := map[string][]string{}
	for _, f := range flags {
		key, labels, err := contract.ParseEntryFlag(f)
		if err != nil {
			return nil, err
		}
		out[key] = labels
	}
	return out, nil
}

func httpEntryKeysFromContract(c *contract.Contract) []string {
	if c == nil {
		return nil
	}
	seen := map[string]struct{}{}
	add := func(k string) {
		if key, ok := contract.CanonicalHTTPEntryKey(k); ok {
			seen[key] = struct{}{}
		}
	}
	for k := range c.HTTPHosts {
		add(k)
	}
	for k := range c.EntryPoints {
		add(k)
	}
	for _, mode := range c.Modes {
		for k := range mode.Ports {
			add(k)
		}
		for k := range mode.HTTPHosts {
			add(k)
		}
		for k := range mode.EntryPoints {
			add(k)
		}
	}
	var out []string
	for _, key := range contract.HTTPEntryKeys {
		if _, ok := seen[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func defaultEntryLabels(key string, c *contract.Contract) []string {
	if c != nil {
		if labels := contract.NormalizeEntryLabels(c.EntryPoints[key]); len(labels) > 0 {
			return labels
		}
		if labels := c.HTTPHosts[key]; len(labels) > 0 && key == "web" {
			return append([]string(nil), labels...)
		}
		for _, name := range contract.ModeNames(c) {
			if labels := c.Modes[name].HTTPHosts[key]; len(labels) > 0 && key == "web" {
				return append([]string(nil), labels...)
			}
		}
		if key == "web" && strings.TrimSpace(c.HostPrefix) != "" {
			return []string{"app"}
		}
	}
	if key == "api" {
		return []string{"api"}
	}
	return []string{key}
}

func printLocalhostHostsNote() {
	println(dim("*.localhost is resolved by the OS and browsers. Do not add those names to the hosts file."))
	println(dim("Siding will not write your hosts file."))
}

// confirmProxyPortFree checks host :80. The siding proxy is designed to bind it.
// Our own siding-proxy-caddy already on :80 is fine (not a conflict).
// Busy: warn and name the occupant. TTY yes → stop init so they can free it.
// TTY no → continue without the proxy. Non-interactive → error.
func confirmProxyPortFree(tty bool) (bool, error) {
	port := state.ProxyHTTPListen
	rep := hostport.Lookup(context.Background(), port)
	blockers := rep.Blocking(proxy.ContainerName)
	return decideProxyPort(tty, port, blockers)
}

func decideProxyPort(tty bool, port int, blockers []hostport.Occupant) (bool, error) {
	if len(blockers) == 0 {
		return true, nil
	}
	println()
	printf("%s\n", red(fmt.Sprintf("Host :%d is already in use. Siding's proxy needs that port.", port)))
	for _, line := range hostport.FormatOccupants(blockers) {
		printf("  • %s\n", yellow(line))
	}
	println(dim("Disable whatever is listed, then run siding init again."))
	if tty {
		if promptYesNo(fmt.Sprintf("Stop init now so you can free port %d?", port), true) {
			return false, fmt.Errorf("init stopped: free port %d, then re-run siding init", port)
		}
		println(yellow("Continuing without the shared proxy (raw published ports)."))
		return false, nil
	}
	return false, hostport.BusyError(port, blockers)
}

func prompt(label, fallback string) string {
	if fallback != "" {
		printf("%s [default: %s]: ", bold(label), dim(fallback))
	} else {
		printf("%s: ", bold(label))
	}
	flushOut()
	line, _ := stdinReader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback
	}
	return line
}

func promptYesNo(label string, defYes bool) bool {
	hint := "Y"
	if !defYes {
		hint = "N"
	}
	printf("%s [default: %s]: ", bold(label), dim(hint))
	flushOut()
	line, _ := stdinReader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return defYes
	}
	return line == "y" || line == "yes"
}

// guessFolderPrefix is the leading org- segment of a clone folder
// (acme-genie2 → acme-). Single-segment names have no prefix.
func guessFolderPrefix(folder string) string {
	base := paths.Basename(folder)
	i := strings.Index(base, "-")
	if i <= 0 || i == len(base)-1 {
		return ""
	}
	return strings.ToLower(base[:i+1])
}

func isNone(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "none", "no", "off", "-":
		return true
	default:
		return false
	}
}
