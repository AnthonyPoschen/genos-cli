package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnthonyPoschen/genos-cli/internal/apiclient"
)

const (
	defaultSavePollInterval = 2 * time.Second
	defaultExportTimeout    = 15 * time.Minute
	defaultImportTimeout    = 30 * time.Minute
	maxSavePollInterval     = time.Minute
	maxSaveTimeout          = 24 * time.Hour
	minSavePollInterval     = 100 * time.Millisecond
)

func (r *runner) saves(args []string) int {
	if len(args) == 0 {
		return r.usage("saves requires a subcommand")
	}
	switch args[0] {
	case "export":
		return r.savesExport(args[1:])
	case "export-status":
		return r.savesExportStatus(args[1:])
	case "import":
		return r.savesImport(args[1:])
	case "import-status":
		return r.savesImportStatus(args[1:])
	case "import-validate":
		return r.savesImportValidate(args[1:])
	case "import-replace":
		return r.savesImportReplace(args[1:])
	case "-h", "--help", "help":
		fmt.Fprint(r.out, usage)
		return 0
	default:
		return r.usage(fmt.Sprintf("unknown saves subcommand %q", args[0]))
	}
}

func (r *runner) savesExport(args []string) int {
	rest, flags, err := parseSavesFlags(args, map[string]bool{
		"--interval": true, "--timeout": true, "--output": true,
	}, map[string]bool{"--wait": true})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("saves export requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	interval, timeout, err := resolvePollTiming(flags, defaultSavePollInterval, defaultExportTimeout)
	if err != nil {
		return r.usage(err.Error())
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Minute)
	defer cancel()

	export, err := client.CreateSaveExport(ctx, rest[0])
	if err != nil {
		return r.fail(err)
	}
	if !flags.bools["--wait"] {
		return r.printJSON(export)
	}

	deadline := time.Now().Add(timeout)
	export, err = client.WaitSaveExport(ctx, rest[0], export.ID, interval, deadline, r.sleep, func(current apiclient.SaveExport) {
		fmt.Fprintf(r.err, "genos: export %s %s %d%%\n", current.ID, current.Status, current.ProgressPercent)
	})
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SaveExportFailed(export.Status) {
		_ = r.printJSON(export)
		return r.fail(fmt.Errorf("save export %s", export.Status))
	}
	if export.Status == "succeeded" && export.DownloadURL != "" {
		fmt.Fprintf(r.err, "genos: downloadURL %s\n", export.DownloadURL)
	}
	if out := flags.values["--output"]; out != "" {
		if export.DownloadURL == "" {
			return r.fail(errors.New("save export succeeded without a downloadURL"))
		}
		if err := client.DownloadToFile(ctx, export.DownloadURL, out); err != nil {
			return r.fail(err)
		}
		fmt.Fprintf(r.err, "genos: wrote %s\n", out)
	}
	return r.printJSON(export)
}

func (r *runner) savesExportStatus(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("saves export-status requires a server id and export id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(args[1]); err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	export, err := client.GetSaveExport(ctx, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SaveExportFailed(export.Status) {
		_ = r.printJSON(export)
		return r.fail(fmt.Errorf("save export %s", export.Status))
	}
	return r.printJSON(export)
}

func (r *runner) savesImport(args []string) int {
	rest, flags, err := parseSavesFlags(args, map[string]bool{
		"--file": true, "--media-type": true, "--interval": true, "--timeout": true,
	}, map[string]bool{
		"--wait": true, "--yes": true, "--apply-save-mods": true, "--no-apply-save-mods": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 1 {
		return r.usage("saves import requires a server id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	filePath := flags.values["--file"]
	if strings.TrimSpace(filePath) == "" {
		return r.usage("saves import requires --file")
	}
	if flags.bools["--apply-save-mods"] && flags.bools["--no-apply-save-mods"] {
		return r.usage("use only one of --apply-save-mods or --no-apply-save-mods")
	}
	mediaType := flags.values["--media-type"]
	if mediaType == "" {
		mediaType = "application/zip"
	}
	interval, timeout, err := resolvePollTiming(flags, defaultSavePollInterval, defaultImportTimeout)
	if err != nil {
		return r.usage(err.Error())
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return r.fail(err)
	}
	if info.IsDir() {
		return r.fail(fmt.Errorf("%s is a directory", filePath))
	}
	fileName := filepath.Base(filePath)

	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Minute)
	defer cancel()

	replacement, err := client.CreateSaveImport(ctx, rest[0], fileName, mediaType, info.Size())
	if err != nil {
		return r.fail(err)
	}
	if replacement.UploadURL == "" {
		return r.fail(errors.New("save import response missing uploadURL"))
	}

	file, err := os.Open(filePath)
	if err != nil {
		return r.fail(err)
	}
	uploadErr := client.UploadSaveArchive(ctx, replacement.UploadURL, "application/zip", file, info.Size())
	_ = file.Close()
	if uploadErr != nil {
		return r.fail(uploadErr)
	}
	fmt.Fprintf(r.err, "genos: uploaded %s (%d bytes)\n", fileName, info.Size())

	if !flags.bools["--wait"] {
		// Refresh after upload so agents see current status when present.
		if refreshed, err := client.GetSaveImport(ctx, rest[0], replacement.ID); err == nil {
			replacement = refreshed
		}
		return r.printJSON(replacement)
	}

	replacement, err = client.ValidateSaveImport(ctx, rest[0], replacement.ID)
	if err != nil {
		return r.fail(err)
	}
	deadline := time.Now().Add(timeout)
	if !apiclient.SaveImportReviewOrFailed(replacement.Status) {
		replacement, err = client.WaitSaveImport(ctx, rest[0], replacement.ID, interval, deadline, r.sleep, apiclient.SaveImportReviewOrFailed, func(current apiclient.SaveImport) {
			fmt.Fprintf(r.err, "genos: import %s %s %d%%\n", current.ID, current.Status, current.ProgressPercent)
		})
		if err != nil {
			return r.fail(err)
		}
	}
	if apiclient.SaveImportFailed(replacement.Status) {
		_ = r.printJSON(replacement)
		return r.fail(fmt.Errorf("save import %s", replacement.Status))
	}
	if !flags.bools["--yes"] {
		_ = r.printJSON(replacement)
		fmt.Fprintf(r.err, "genos: import is ready; re-run with --yes to replace, or: genos saves import-replace %s %s --yes\n", rest[0], replacement.ID)
		return 1
	}

	applyMods := applySaveModsFromFlags(flags)
	replacement, err = client.ReplaceSaveImport(ctx, rest[0], replacement.ID, true, applyMods)
	if err != nil {
		return r.fail(err)
	}
	replacement, err = client.WaitSaveImport(ctx, rest[0], replacement.ID, interval, deadline, r.sleep, apiclient.SaveImportTerminal, func(current apiclient.SaveImport) {
		line := fmt.Sprintf("genos: import %s %s %d%%", current.ID, current.Status, current.ProgressPercent)
		if current.Recovery != "" {
			line += " recovery=" + current.Recovery
		}
		fmt.Fprintln(r.err, line)
	})
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SaveImportFailed(replacement.Status) {
		_ = r.printJSON(replacement)
		return r.fail(fmt.Errorf("save import %s", replacement.Status))
	}
	return r.printJSON(replacement)
}

func (r *runner) savesImportStatus(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("saves import-status requires a server id and import id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(args[1]); err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	replacement, err := client.GetSaveImport(ctx, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	if apiclient.SaveImportFailed(replacement.Status) {
		_ = r.printJSON(replacement)
		return r.fail(fmt.Errorf("save import %s", replacement.Status))
	}
	return r.printJSON(replacement)
}

func (r *runner) savesImportValidate(args []string) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if len(args) != 2 || strings.HasPrefix(args[0], "-") || strings.HasPrefix(args[1], "-") {
		return r.usage("saves import-validate requires a server id and import id")
	}
	if err := validateID(args[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(args[1]); err != nil {
		return r.fail(err)
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	replacement, err := client.ValidateSaveImport(ctx, args[0], args[1])
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(replacement)
}

func (r *runner) savesImportReplace(args []string) int {
	rest, flags, err := parseSavesFlags(args, nil, map[string]bool{
		"--yes": true, "--apply-save-mods": true, "--no-apply-save-mods": true,
	})
	if errors.Is(err, errHelp) {
		fmt.Fprint(r.out, usage)
		return 0
	}
	if err != nil {
		return r.usage(err.Error())
	}
	if len(rest) != 2 {
		return r.usage("saves import-replace requires a server id and import id")
	}
	if err := validateID(rest[0]); err != nil {
		return r.fail(err)
	}
	if err := validateID(rest[1]); err != nil {
		return r.fail(err)
	}
	if flags.bools["--apply-save-mods"] && flags.bools["--no-apply-save-mods"] {
		return r.usage("use only one of --apply-save-mods or --no-apply-save-mods")
	}
	if !flags.bools["--yes"] {
		return r.usage("saves import-replace requires --yes (destructive replace)")
	}
	client, err := r.api()
	if err != nil {
		return r.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	replacement, err := client.ReplaceSaveImport(ctx, rest[0], rest[1], true, applySaveModsFromFlags(flags))
	if err != nil {
		return r.fail(err)
	}
	return r.printJSON(replacement)
}

type savesFlagSet struct {
	values map[string]string
	bools  map[string]bool
}

func parseSavesFlags(args []string, valued, switches map[string]bool) (rest []string, flags savesFlagSet, err error) {
	flags.values = map[string]string{}
	flags.bools = map[string]bool{}
	if valued == nil {
		valued = map[string]bool{}
	}
	if switches == nil {
		switches = map[string]bool{}
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch arg {
		case "-h", "--help":
			return nil, flags, errHelp
		case "--":
			return append(rest, args[index+1:]...), flags, nil
		}
		if switches[arg] {
			if flags.bools[arg] {
				return nil, flags, fmt.Errorf("%s specified more than once", arg)
			}
			flags.bools[arg] = true
			continue
		}
		if valued[arg] {
			if _, exists := flags.values[arg]; exists {
				return nil, flags, fmt.Errorf("%s specified more than once", arg)
			}
			if index+1 >= len(args) {
				return nil, flags, fmt.Errorf("%s requires a value", arg)
			}
			index++
			flags.values[arg] = args[index]
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return nil, flags, fmt.Errorf("unknown flag %s", arg)
		}
		rest = append(rest, arg)
	}
	return rest, flags, nil
}

func applySaveModsFromFlags(flags savesFlagSet) *bool {
	if flags.bools["--apply-save-mods"] {
		v := true
		return &v
	}
	if flags.bools["--no-apply-save-mods"] {
		v := false
		return &v
	}
	return nil
}

func resolvePollTiming(flags savesFlagSet, defaultInterval, defaultTimeout time.Duration) (interval, timeout time.Duration, err error) {
	interval = defaultInterval
	timeout = defaultTimeout
	if raw, ok := flags.values["--interval"]; ok {
		interval, err = time.ParseDuration(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("--interval must be a duration (e.g. 2s)")
		}
	}
	if raw, ok := flags.values["--timeout"]; ok {
		timeout, err = time.ParseDuration(raw)
		if err != nil {
			return 0, 0, fmt.Errorf("--timeout must be a duration (e.g. 15m)")
		}
	}
	if interval < minSavePollInterval {
		return 0, 0, fmt.Errorf("--interval must be at least %s", minSavePollInterval)
	}
	if interval > maxSavePollInterval {
		return 0, 0, fmt.Errorf("--interval must be at most %s", maxSavePollInterval)
	}
	if timeout <= 0 || timeout > maxSaveTimeout {
		return 0, 0, fmt.Errorf("--timeout must be between 1ns and %s", maxSaveTimeout)
	}
	return interval, timeout, nil
}

func (r *runner) sleep(d time.Duration) {
	if r.opts.Sleep != nil {
		r.opts.Sleep(d)
		return
	}
	time.Sleep(d)
}
