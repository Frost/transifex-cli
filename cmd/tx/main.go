package tx

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/transifex/cli/internal/txlib"
	"github.com/transifex/cli/internal/txlib/config"
	"github.com/transifex/cli/pkg/jsonapi"
	"github.com/urfave/cli/v2"
)

func Main() {
	errorColor := color.New(color.FgRed).SprintfFunc()
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Println("TX Client, version=" + c.App.Version)
	}
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "root-config",
			Usage: "Root configuration from `FILE`",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Load configuration from `FILE`",
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The api token to use",
			EnvVars: []string{"TX_TOKEN"},
		},
		&cli.StringFlag{
			Name:    "hostname",
			Aliases: []string{"H"},
			Usage:   "The API hostname",
			EnvVars: []string{"TX_HOSTNAME"},
		},
		&cli.StringFlag{
			Name:    "cacert",
			Usage:   "Path to CA certificate bundle file",
			EnvVars: []string{"TX_CACERT"},
		},
	}
	app := &cli.App{
		Version:                txlib.Version,
		UseShortOptionHandling: true,
		Commands: []*cli.Command{
			{
				Name:    "migrate",
				Aliases: []string{"mg"},
				Usage:   "Migrate legacy configuration.",
				Action: func(c *cli.Context) error {
					// Load current config
					cfg, err := config.LoadFromPaths(
						c.String("root-config"), c.String("config"))
					if err != nil {
						return cli.Exit(err, 1)
					}

					client, err := txlib.GetClient(c.String("cacert"))
					if err != nil {
						return cli.Exit(err, 1)
					}

					api := jsonapi.Connection{
						Client: client,
					}

					backUpFilePath, err := txlib.MigrateLegacyConfigFile(&cfg,
						api)

					if err != nil {
						return cli.Exit(err, 1)
					}
					fmt.Printf(
						"Migration ended! We have also created a backup "+
							"file for your previous config file `%s`.\n",
						backUpFilePath,
					)
					return nil
				},
			},
			{
				Name:  "push",
				Usage: "tx push [options] [resource_id...]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "source",
						Usage:   "Push the source file",
						Aliases: []string{"s"},
					},
					&cli.BoolFlag{
						Name:    "translation",
						Usage:   "Push the translation files",
						Aliases: []string{"t"},
					},
					&cli.BoolFlag{
						Name:    "force",
						Usage:   "Push source files without checking modification times",
						Aliases: []string{"f"},
					},
					&cli.BoolFlag{
						Name:  "skip",
						Usage: "Whether to skip on errors",
					},
					&cli.BoolFlag{
						Name:  "xliff",
						Usage: "Whether to push XLIFF files",
					},
					&cli.BoolFlag{
						Name: "use-git-timestamps",
						Usage: "Compare local files to their Transifex " +
							"version by their latest commit timestamps. Use " +
							"this option, for example, when cloning a Git " +
							"repository.",
					},
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Usage: "Whether to create missing languages on the " +
							"remote server when possible",
					},
					&cli.StringFlag{
						Name:    "languages",
						Aliases: []string{"l"},
						Usage: "Specify which languages you want to push " +
							"translations for",
					},
					&cli.StringFlag{
						Name:    "resources",
						Aliases: []string{"r"},
						Usage: "Specify which resources you want to push " +
							"the translations",
					},
					&cli.StringFlag{
						Name: "branch",
						Usage: "Push to specific branch (use empty argument " +
							"'' to use the current branch, if it can be " +
							"determined)",
						Value: "-1",
					},
					&cli.IntFlag{
						Name:    "workers",
						Usage:   "How many parallel workers to use",
						Aliases: []string{"w"},
						Value:   5,
					},
				},
				Action: func(c *cli.Context) error {
					cfg, err := config.LoadFromPaths(
						c.String("root-config"),
						c.String("config"),
					)
					if err != nil {
						return cli.Exit(
							errorColor(
								"Error loading configuration: %s",
								err,
							),
							1,
						)
					}
					hostname, token, err := txlib.GetHostAndToken(
						&cfg, c.String("hostname"), c.String("token"),
					)
					if err != nil {
						return cli.Exit(
							errorColor(
								"Error getting API token: %s",
								err,
							),
							1,
						)
					}

					client, err := txlib.GetClient(c.String("cacert"))
					if err != nil {
						return cli.Exit(
							errorColor(
								"Error getting HTTP client configuration: %s",
								err,
							),
							1,
						)
					}

					api := jsonapi.Connection{
						Host:   hostname,
						Token:  token,
						Client: client,
						Headers: map[string]string{
							"Integration": "txclient",
						},
					}

					resourceIds := c.Args().Slice()
					if c.String("resources") != "" {
						extraResourceIds := strings.Split(
							c.String("resources"),
							",",
						)
						resourceIds = append(resourceIds, extraResourceIds...)
					}

					var languages []string
					if c.String("languages") != "" {
						languages = strings.Split(c.String("languages"), ",")
					}
					args := txlib.PushCommandArguments{
						Source:           c.Bool("source"),
						Translation:      c.Bool("translation"),
						Force:            c.Bool("force"),
						Skip:             c.Bool("skip"),
						Xliff:            c.Bool("xliff"),
						Languages:        languages,
						ResourceIds:      resourceIds,
						UseGitTimestamps: c.Bool("use-git-timestamps"),
						Branch:           c.String("branch"),
						All:              c.Bool("all"),
						Workers:          c.Int("workers"),
					}

					if args.All && len(args.Languages) > 0 {
						return cli.Exit(errorColor(
							"It doesn't make sense to use the '--all' flag "+
								"with the '--language' flag",
						), 1)
					}

					if !args.Translation &&
						(args.All || len(args.Languages) > 0) {
						return cli.Exit(errorColor(
							"It doesn't make sense to use the '--all' or "+
								"'--language' flag without the "+
								"'--translation' flag",
						), 1)
					}

					if args.Force && args.UseGitTimestamps {
						return cli.Exit(errorColor(
							"It doesn't make sense to use the '--force' "+
								"flag with the '--use-git-timestamps' flag",
						), 1)
					}

					if args.Xliff && !args.Translation {
						return cli.Exit(errorColor(
							"--xliff only makes sense when used with "+
								"`-t/--translation`",
						), 1)
					}

					if 1 > args.Workers || args.Workers > 30 {
						return cli.Exit(errorColor(
							"Please choose a number of workers between 1 and 30",
						), 1)
					}

					err = txlib.PushCommand(&cfg, api, args)
					if err != nil {
						return cli.Exit("", 1)
					}
					return nil
				},
			},
			{
				Name:  "pull",
				Usage: "tx pull [options] [resource_id...]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "xliff",
						Usage: `Download translation files in xliff format`,
					},
					&cli.BoolFlag{
						Name:  "json",
						Usage: `Download translation files in json format`,
					}, &cli.StringFlag{
						Name:    "content_encoding",
						Aliases: []string{"e"},
						Value:   "text",
						Usage: "The encoding of the file. This can be one " +
							"of the following:\n    'text', 'base64'",
					},
					&cli.StringFlag{
						Name:    "mode",
						Aliases: []string{"m"},
						Value:   "default",
						Usage: "The translation mode of the downloaded " +
							"file. This can be one of the following:\n    " +
							"'default', 'reviewed', 'proofread', " +
							"'translator', 'untranslated',\n    " +
							"'onlytranslated', 'onlyreviewed', " +
							"'onlyproofread', 'sourceastranslation'",
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage: "Force the download of the translations" +
							"files regardless of whether timestamps on the " +
							"local computer are newer than those on the server",
					},
					&cli.StringFlag{
						Name:    "languages",
						Value:   "",
						Aliases: []string{"l"},
						Usage: "Download specific languages, comma " +
							"separated Transifex language codes",
					},
					&cli.BoolFlag{
						Name:    "source",
						Aliases: []string{"s"},
						Usage:   "Download source file only",
					},
					&cli.BoolFlag{
						Name:    "translations",
						Aliases: []string{"t"},
						Usage:   "Downloads translations files (default)",
					},
					&cli.BoolFlag{
						Name:    "disable-overwrite",
						Aliases: []string{"d"},
						Usage:   "Whether skip existing files",
					},
					&cli.BoolFlag{
						Name:  "skip",
						Usage: "Whether to skip on errors",
					},
					&cli.BoolFlag{
						Name: "use-git-timestamps",
						Usage: "Compare local files to their Transifex " +
							"version by their latest commit timestamps. Use " +
							"this option, for example, when cloning a Git " +
							"repository.",
					},
					&cli.StringFlag{
						Name: "branch",
						Usage: "Push to specific branch (use empty argument " +
							"'' to use the current branch, if it can be " +
							"determined)",
						Value: "-1",
					},
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Usage:   "Whether to download all files",
					},
					&cli.StringFlag{
						Name:    "resources",
						Aliases: []string{"r"},
						Usage: "Backwards compatibility with old client " +
							"to fetch resource ids",
					},
					&cli.IntFlag{
						Name: "minimum-perc",
						Usage: "Specify the minimum acceptable percentage of " +
							"a translation mode in order to download it.",
						Value: -1,
					},
					&cli.IntFlag{
						Name:    "workers",
						Usage:   "How many parallel workers to use",
						Aliases: []string{"w"},
						Value:   5,
					},
				},
				Action: func(c *cli.Context) error {
					cfg, err := config.LoadFromPaths(c.String("root-config"),
						c.String("config"))
					if err != nil {
						return err
					}

					hostname, token, err := txlib.GetHostAndToken(
						&cfg, c.String("hostname"), c.String("token"),
					)
					if err != nil {
						return err
					}

					client, err := txlib.GetClient(c.String("cacert"))
					if err != nil {
						return err
					}
					api := jsonapi.Connection{
						Host:   hostname,
						Token:  token,
						Client: client,
						Headers: map[string]string{
							"Integration": "txclient",
						},
					}

					resourceIds := c.Args().Slice()
					if c.String("resources") != "" {
						extraResourceIds := strings.Split(
							c.String("resources"),
							",",
						)
						resourceIds = append(resourceIds, extraResourceIds...)
					}

					arguments := txlib.PullCommandArguments{
						ContentEncoding:   c.String("content_encoding"),
						Mode:              c.String("mode"),
						Force:             c.Bool("force"),
						Skip:              c.Bool("skip"),
						Source:            c.Bool("source"),
						Translations:      c.Bool("translations"),
						DisableOverwrite:  c.Bool("disable-overwrite"),
						All:               c.Bool("all"),
						ResourceIds:       resourceIds,
						UseGitTimestamps:  c.Bool("use-git-timestamps"),
						Branch:            c.String("branch"),
						MinimumPercentage: c.Int("minimum-perc"),
						Workers:           c.Int("workers"),
					}

					if c.Bool("xliff") && c.Bool("json") {
						return cli.Exit(errorColor(
							"You cannot use both flags '%s' and '%s'.",
							"xliff", "json",
						), 1)
					} else if c.Bool("xliff") {
						arguments.FileType = "xliff"
					} else if c.Bool("json") && c.Bool("source") {
						return cli.Exit(errorColor(
							"You cannot use both flags '%s' and '%s'. "+
								"Source files do not support json format.",
							"json", "source",
						), 1)
					} else if c.Bool("json") {
						arguments.FileType = "json"
					} else {
						arguments.FileType = "default"
					}

					if c.String("languages") != "" && c.Bool("all") {
						return cli.Exit(errorColor(
							"You cannot use both flags '%s' and '%s'.",
							"languages", "all",
						), 1)
					}

					if c.String("languages") != "" {
						arguments.Languages = append(
							arguments.Languages,
							strings.Split(c.String("languages"), ",")...,
						)
					}

					if 1 > arguments.Workers || arguments.Workers > 30 {
						return cli.Exit(errorColor(
							"Please choose a number of workers between 1 and 30",
						), 1)
					}

					err = txlib.PullCommand(&cfg, &api, &arguments)
					if err != nil {
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:    "add",
				Aliases: []string{"a"},
				Usage: "Add a resource in config. Use no arguments for " +
					"an interactive mode.",
				Action: func(c *cli.Context) error {
					cfg, err := config.LoadFromPaths(
						c.String("root-config"), c.String("config"))
					if err != nil {
						return fmt.Errorf(
							"something went wrong while loading the "+
								"configuration file. %w",
							err,
						)
					}
					if cfg.Local == nil {
						return errors.New(
							"please create a local configuration file in " +
								"order to continue",
						)
					}

					flagList := []string{
						"organization",
						"project",
						"resource",
						"file-filter",
						"type",
					}
					var missingFlags []string
					for _, value := range flagList {
						if c.String(value) == "" {
							missingFlags = append(missingFlags, value)
						}
					}

					sourceFile := c.Args().First()
					missingFlagsCount := len(missingFlags)
					var args = txlib.AddCommandArguments{
						OrganizationSlug: c.String("organization"),
						ProjectSlug:      c.String("project"),
						ResourceSlug:     c.String("resource"),
						FileFilter:       c.String("file-filter"),
						RType:            c.String("type"),
						SourceFile:       sourceFile,
					}
					if missingFlagsCount == 0 {
						return txlib.AddCommand(
							&cfg,
							&args,
						)
					}

					if missingFlagsCount == len(flagList) {
						hostname, token, err := txlib.GetHostAndToken(
							&cfg, c.String("hostname"), c.String("token"),
						)
						if err != nil {
							return cli.Exit(err, 1)
						}
						api := jsonapi.Connection{
							Host:  hostname,
							Token: token,
							Headers: map[string]string{
								"Integration": "txclient",
							},
						}
						err = txlib.AddCommandInteractive(&cfg, api)
						if err != nil {
							if err == promptui.ErrInterrupt {
								return cli.Exit("", 1)
							} else {
								return cli.Exit(
									errorColor(fmt.Sprint(err)),
									1)
							}

						}
					}

					if missingFlagsCount >= 1 &&
						missingFlagsCount < len(flagList) {
						err := cli.ShowCommandHelp(c, "add")
						if err != nil {
							return cli.Exit(err, 1)
						}
						if missingFlagsCount == 1 {
							return fmt.Errorf("flag %s is not set",
								strings.Join(missingFlags, ","))
						} else {
							return fmt.Errorf("flags %s are not set",
								strings.Join(missingFlags, ","))
						}
					}

					return nil
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "organization",
						Usage: "The organization slug for the project",
					},
					&cli.StringFlag{
						Name:  "project",
						Usage: "The project slug",
					},
					&cli.StringFlag{
						Name:  "resource",
						Usage: "The resource slug",
					},
					&cli.StringFlag{
						Name: "file-filter",
						Usage: "Path expression pointing to the location of " +
							"the translation files",
					},
					&cli.StringFlag{
						Name:  "type",
						Usage: "The file format type of your resource",
					},
				},
			},
			{
				Name:  "init",
				Usage: "tx init",
				Action: func(c *cli.Context) error {
					err := txlib.InitCommand()
					if err != nil {
						return cli.Exit(errorColor(fmt.Sprint(err)), 1)
					}
					return nil
				},
			},
			{
				Name:  "delete",
				Usage: "tx delete [options] [resource_id...]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "resources",
						Aliases: []string{"r"},
						Usage:   "Resource ids to delete",
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage: "Whether to continue if there are " +
							"translations in the resources",
					},
					&cli.BoolFlag{
						Name:    "skip",
						Aliases: []string{"s"},
						Usage:   "Whether to skip on errors",
					},
					&cli.StringFlag{
						Name: "branch",
						Usage: "Delete specific branch (use empty argument " +
							"'' to use the current branch, if it can be " +
							"determined)",
						Value: "-1",
					},
				},
				Action: func(c *cli.Context) error {
					cfg, err := config.LoadFromPaths(c.String("root-config"),
						c.String("config"))
					if err != nil {
						return err
					}

					hostname, token, err := txlib.GetHostAndToken(
						&cfg, c.String("hostname"), c.String("token"),
					)
					if err != nil {
						return err
					}

					client, err := txlib.GetClient(c.String("cacert"))
					if err != nil {
						return err
					}

					api := jsonapi.Connection{
						Host:   hostname,
						Token:  token,
						Client: client,
						Headers: map[string]string{
							"Integration": "txclient",
						},
					}

					// Get extra resource ids
					resourceIds := c.Args().Slice()
					if c.String("resources") != "" {
						extraResourceIds := strings.Split(
							c.String("resources"),
							",",
						)
						resourceIds = append(resourceIds, extraResourceIds...)
					}

					// Construct arguments
					arguments := txlib.DeleteCommandArguments{
						ResourceIds: resourceIds,
						Force:       c.Bool("force"),
						Skip:        c.Bool("skip"),
						Branch:      c.String("branch"),
					}
					// Proceed with deletion
					err = txlib.DeleteCommand(&cfg, api, &arguments)
					if err != nil {
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
			{
				Name:  "update",
				Usage: "Update the `tx` application if there is a newer version",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "check",
						Aliases: []string{"c"},
						Usage:   "Check if there is a new version of tx",
					},
					&cli.BoolFlag{
						Name:    "no-interactive",
						Aliases: []string{"ni"},
						Usage:   "Update if there is a newer version without prompt",
					},
					&cli.BoolFlag{
						Name:    "debug",
						Aliases: []string{"d"},
						Usage:   "Enable debug logs for the update process",
					},
				},
				Action: func(c *cli.Context) error {
					version := c.App.Version
					arguments := txlib.UpdateCommandArguments{
						Version:       version,
						Check:         c.Bool("check"),
						NoInteractive: c.Bool("no-interactive"),
						Debug:         c.Bool("debug"),
					}

					err := txlib.UpdateCommand(arguments)
					if err != nil {
						if err == promptui.ErrInterrupt {
							return cli.Exit("", 1)
						} else {
							return cli.Exit(errorColor(fmt.Sprint(err)), 1)
						}
					}
					return nil
				},
			},
			{
				Name:  "status",
				Usage: "tx status [resource_id...]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "resources",
						Aliases: []string{"r"},
						Usage: "Resource ids to get status for that are " +
							"included in your config file",
					},
				},
				Action: func(c *cli.Context) error {
					cfg, err := config.LoadFromPaths(c.String("root-config"),
						c.String("config"))
					if err != nil {
						return err
					}

					hostname, token, err := txlib.GetHostAndToken(
						&cfg, c.String("hostname"), c.String("token"),
					)
					if err != nil {
						return err
					}

					client, err := txlib.GetClient(c.String("cacert"))
					if err != nil {
						return err
					}

					api := jsonapi.Connection{
						Host:   hostname,
						Token:  token,
						Client: client,
						Headers: map[string]string{
							"Integration": "txclient",
						},
					}

					// Get extra resource ids
					resourceIds := c.Args().Slice()
					if c.String("resources") != "" {
						extraResourceIds := strings.Split(
							c.String("resources"),
							",",
						)
						resourceIds = append(resourceIds, extraResourceIds...)
					}

					// Construct arguments
					arguments := txlib.StatusCommandArguments{
						ResourceIds: resourceIds,
					}
					// Proceed with deletion
					err = txlib.StatusCommand(&cfg, api, &arguments)
					if err != nil {
						return cli.Exit(err, 1)
					}
					return nil
				},
			},
		},
		Flags: flags,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
