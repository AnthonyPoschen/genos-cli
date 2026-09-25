package cli

import (
	"github.com/spf13/cobra"
)

func newServerModsCmd(r *runner) *cobra.Command {
	return newModsGroupCmd(r, serverModsTarget, "serverID")
}

func newServerSavesCmd(r *runner) *cobra.Command {
	return newSavesGroupCmd(r, serverSavesTarget, "serverID")
}

func newProfileModsCmd(r *runner) *cobra.Command {
	return newModsGroupCmd(r, profileModsTarget, "profileID")
}

func newProfileSavesCmd(r *runner) *cobra.Command {
	return newSavesGroupCmd(r, profileSavesTarget, "profileID")
}

func newModsGroupCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "mods",
		Short: "Manage mods for a " + target.noun,
	})
	cmd.AddCommand(
		newModsListCmd(r, target, idName),
		newModsSearchCmd(r, target, idName),
		newModsShowCmd(r, target, idName),
		newModsCredentialsCmd(r, target, idName),
		newModsCredentialsClearCmd(r, target, idName),
		newModsStageCmd(r, target, idName),
		newModsUnstageCmd(r, target, idName),
		newModsDiscardCmd(r, target, idName),
		newModsApplyCmd(r, target, idName),
		newModsDraftCmd(r, target, idName),
		newModsImportCmd(r, target, idName),
		newModsDraftApplyCmd(r, target, idName),
	)
	return cmd
}

func newModsListCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	return &cobra.Command{
		Use:   "list <" + idName + ">",
		Short: "List mod state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.modsList(target, args))
		},
	}
}

func newModsSearchCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var query, category, sort string
	var page, pageSize string
	cmd := &cobra.Command{
		Use:   "search <" + idName + ">",
		Short: "Search the mod catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--query", query, flagChanged(cmd, "query"))
			rebuilt = appendFlag(rebuilt, "--category", category, flagChanged(cmd, "category"))
			rebuilt = appendFlag(rebuilt, "--sort", sort, flagChanged(cmd, "sort"))
			rebuilt = appendFlag(rebuilt, "--page", page, flagChanged(cmd, "page"))
			rebuilt = appendFlag(rebuilt, "--page-size", pageSize, flagChanged(cmd, "page-size"))
			return codeErr(r.modsSearch(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Search query")
	cmd.Flags().StringVar(&category, "category", "", "Category filter")
	cmd.Flags().StringVar(&sort, "sort", "", "Sort order")
	cmd.Flags().StringVar(&page, "page", "", "Page number")
	cmd.Flags().StringVar(&pageSize, "page-size", "", "Page size")
	return cmd
}

func newModsShowCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <" + idName + "> <providerModID>",
		Short: "Show catalog detail for one mod",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.modsShow(target, args))
		},
	}
}

func newModsCredentialsCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var username, token, expectedSetup string
	cmd := &cobra.Command{
		Use:   "credentials <" + idName + ">",
		Short: "Set mod provider credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--username", username, flagChanged(cmd, "username"))
			rebuilt = appendFlag(rebuilt, "--token", token, flagChanged(cmd, "token"))
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsCredentials(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "Provider username")
	cmd.Flags().StringVar(&token, "token", "", "Provider token")
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsCredentialsClearCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var expectedSetup string
	cmd := &cobra.Command{
		Use:   "credentials-clear <" + idName + ">",
		Short: "Clear mod provider credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsCredentialsClear(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsStageCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var provider, expectedSetup string
	cmd := &cobra.Command{
		Use:   "stage <" + idName + "> <providerModID>",
		Short: "Stage a mod for apply",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--provider", provider, flagChanged(cmd, "provider"))
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsStage(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Mod provider id (required)")
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsUnstageCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var expectedSetup string
	cmd := &cobra.Command{
		Use:   "unstage <" + idName + ">",
		Short: "Clear the staged mod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsUnstage(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsDiscardCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var expectedSetup string
	cmd := &cobra.Command{
		Use:   "discard <" + idName + ">",
		Short: "Discard pending mod changes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsDiscard(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsApplyCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var stageID, expectedSetup string
	cmd := &cobra.Command{
		Use:   "apply <" + idName + ">",
		Short: "Apply the staged mod",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--stage-id", stageID, flagChanged(cmd, "stage-id"))
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsApply(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&stageID, "stage-id", "", "Staged stageID (default: autofill)")
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsDraftCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var provider, expectedSetup string
	var modIDs []string
	cmd := &cobra.Command{
		Use:   "draft <" + idName + ">",
		Short: "Replace the collection draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--provider", provider, flagChanged(cmd, "provider"))
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			for _, id := range modIDs {
				rebuilt = append(rebuilt, "--mod-id", id)
			}
			return codeErr(r.modsDraft(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "", "Mod provider id (required)")
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	cmd.Flags().StringArrayVar(&modIDs, "mod-id", nil, "Direct mod id (repeatable)")
	return cmd
}

func newModsImportCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var file, expectedSetup string
	cmd := &cobra.Command{
		Use:   "import <" + idName + ">",
		Short: "Import mod-list.json from --file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--file", file, flagChanged(cmd, "file"))
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			return codeErr(r.modsImport(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "mod-list.json path (default: stdin)")
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	return cmd
}

func newModsDraftApplyCmd(r *runner, target modsTarget, idName string) *cobra.Command {
	var expectedSetup, expectedRevision string
	cmd := &cobra.Command{
		Use:   "draft-apply <" + idName + ">",
		Short: "Apply the collection draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--expected-setup", expectedSetup, flagChanged(cmd, "expected-setup"))
			rebuilt = appendFlag(rebuilt, "--expected-revision", expectedRevision, flagChanged(cmd, "expected-revision"))
			return codeErr(r.modsDraftApply(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&expectedSetup, "expected-setup", "", "CAS expectedSetupID")
	cmd.Flags().StringVar(&expectedRevision, "expected-revision", "", "CAS draft revision")
	return cmd
}

func newSavesGroupCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	cmd := markContainer(&cobra.Command{
		Use:   "saves",
		Short: "Import and export saves for a " + target.noun,
	})
	cmd.AddCommand(
		newSavesExportCmd(r, target, idName),
		newSavesExportStatusCmd(r, target, idName),
		newSavesImportCmd(r, target, idName),
		newSavesImportStatusCmd(r, target, idName),
		newSavesImportValidateCmd(r, target, idName),
		newSavesImportReplaceCmd(r, target, idName),
	)
	return cmd
}

func newSavesExportCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	var wait bool
	var interval, timeout, output string
	cmd := &cobra.Command{
		Use:   "export <" + idName + ">",
		Short: "Start a save export",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendSwitch(rebuilt, "--wait", wait)
			rebuilt = appendFlag(rebuilt, "--interval", interval, flagChanged(cmd, "interval"))
			rebuilt = appendFlag(rebuilt, "--timeout", timeout, flagChanged(cmd, "timeout"))
			rebuilt = appendFlag(rebuilt, "--output", output, flagChanged(cmd, "output"))
			return codeErr(r.savesExport(target, rebuilt))
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "Poll until terminal status")
	cmd.Flags().StringVar(&interval, "interval", "", "Poll interval (default 2s)")
	cmd.Flags().StringVar(&timeout, "timeout", "", "Poll timeout (default 15m)")
	cmd.Flags().StringVar(&output, "output", "", "Download archive to this path on success")
	return cmd
}

func newSavesExportStatusCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	return &cobra.Command{
		Use:   "export-status <" + idName + "> <exportID>",
		Short: "Show save export status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.savesExportStatus(target, args))
		},
	}
}

func newSavesImportCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	var wait, yes, applyMods, noApplyMods bool
	var file, mediaType, interval, timeout string
	cmd := &cobra.Command{
		Use:   "import <" + idName + ">",
		Short: "Upload and optionally replace a save",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendFlag(rebuilt, "--file", file, flagChanged(cmd, "file"))
			rebuilt = appendFlag(rebuilt, "--media-type", mediaType, flagChanged(cmd, "media-type"))
			rebuilt = appendFlag(rebuilt, "--interval", interval, flagChanged(cmd, "interval"))
			rebuilt = appendFlag(rebuilt, "--timeout", timeout, flagChanged(cmd, "timeout"))
			rebuilt = appendSwitch(rebuilt, "--wait", wait)
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			rebuilt = appendSwitch(rebuilt, "--apply-save-mods", applyMods)
			rebuilt = appendSwitch(rebuilt, "--no-apply-save-mods", noApplyMods)
			return codeErr(r.savesImport(target, rebuilt))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Zip archive path (required)")
	cmd.Flags().StringVar(&mediaType, "media-type", "", "Upload media type (default application/zip)")
	cmd.Flags().StringVar(&interval, "interval", "", "Poll interval")
	cmd.Flags().StringVar(&timeout, "timeout", "", "Poll timeout (default 30m)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Validate/poll; with --yes also replace")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm destructive replace")
	cmd.Flags().BoolVar(&applyMods, "apply-save-mods", false, "Apply save mods on replace")
	cmd.Flags().BoolVar(&noApplyMods, "no-apply-save-mods", false, "Do not apply save mods on replace")
	return cmd
}

func newSavesImportStatusCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	return &cobra.Command{
		Use:   "import-status <" + idName + "> <importID>",
		Short: "Show save import status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.savesImportStatus(target, args))
		},
	}
}

func newSavesImportValidateCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	return &cobra.Command{
		Use:   "import-validate <" + idName + "> <importID>",
		Short: "Validate an uploaded save import",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(r.savesImportValidate(target, args))
		},
	}
}

func newSavesImportReplaceCmd(r *runner, target savesTarget, idName string) *cobra.Command {
	var yes, applyMods, noApplyMods bool
	cmd := &cobra.Command{
		Use:   "import-replace <" + idName + "> <importID>",
		Short: "Replace the live save (destructive)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			rebuilt := append([]string{}, args...)
			rebuilt = appendSwitch(rebuilt, "--yes", yes)
			rebuilt = appendSwitch(rebuilt, "--apply-save-mods", applyMods)
			rebuilt = appendSwitch(rebuilt, "--no-apply-save-mods", noApplyMods)
			return codeErr(r.savesImportReplace(target, rebuilt))
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm destructive replace (required)")
	cmd.Flags().BoolVar(&applyMods, "apply-save-mods", false, "Apply save mods")
	cmd.Flags().BoolVar(&noApplyMods, "no-apply-save-mods", false, "Do not apply save mods")
	return cmd
}
