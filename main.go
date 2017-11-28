// Backup tool for Grafana.
// Copyright (C) 2016-2017  Alexander I.Grafov <siberian@laika.name>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//
// ॐ तारे तुत्तारे तुरे स्व

package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/grafana-tools/sdk"
	"path/filepath"
)

// Interesting things to check out from the release notes
// Support loading flags from files (ParseWithFileExpansion()). Use @FILE as an argument.
// Add an Enum() value, allowing only one of a set of values to be selected. eg. Flag(...).Enum("debug", "info", "warning").

// kingpin version of the flags
var (
	app      = kingpin.New("grafana-backup", "A backup tool for Grafana.\n" +
		"Copyright (C) 2016-2017  Alexander I.Grafov <siberian@laika.name>\n" +
		"This program comes with ABSOLUTELY NO WARRANTY.\n" +
		"This is free software, and you are welcome to redistribute it\n" +
		"under conditions of GNU GPL license v3.\n" +
		"" +
		"Call 'grafana-backup help <command>' for details about the command.\n")


	// ## Commands ##

	// Create a sub command for each function. Note: A help sub command is implicitly created by kingpin
	backupCMD        = app.Command("backup", "Back up objects from the remote server to local JSON files.")
	restoreCMD       = app.Command("restore", "Restore objects from local JSON files to the remote server.")
	lsCMD            = app.Command("ls", "List objects on the remote server.")
	lsFilesCMD       = app.Command("ls-files", "List objects contained in local JSON files.")
	configSetCMD     = app.Command("config-set", "Restore a configuration backup to the remote server. NOT YET IMPLEMENTED!")
	configGetCMD     = app.Command("config-get", "Get the configuration of the remote server. NOT YET IMPLEMENTED!")

	// These are mentioned in the help text but don't actually exist.
	//info
	//config
	//help.

	// ## Common Flags ##
	// Connection flags.
	flagServerURL    = app.Flag("url", "URL of Grafana server (defaults to $GRAFANA_URL if not provided)").Required().Short('u').Envar("GRAFANA_URL").URL()
	flagServerKey    = app.Flag("key", "API key of Grafana server (defaults to $GRAFANA_TOKEN if not provided)").Required().Short('k').Envar("GRAFANA_TOKEN").String()
	flagTimeout      = app.Flag("timeout", "The timeout for making requests to the Grafana API.").Default("6m").Short('d').Duration()

	//.HintOptions for command completion only works with the long form of the argument.
	// Dashboard matching flags.
	flagTags         = app.Flag("tag", "Dashboard should match all these tags.").HintOptions("dbserver", "storage", "webserver").Short('t').Strings()
	flagBoardTitle   = app.Flag("title", "Dashboard title should match name.").Short('T').String()
	flagStarred      = app.Flag("starred", "Only match starred dashboards.").Short('s').Bool() // No default for bools? Let's find out.

	// Common flags.
	flagApplyFor     = app.Flag("apply-for", "The type of object to apply the operation for.").Short('a').Default("auto").Enum("auto", "all", "dashboards", "datasources", "users")
	flagForce        = app.Flag("force", "Force overwrite of existing objects.").Short('f').Bool()
	flagVerbose      = app.Flag("verbose", "Verbose output.").Short('v').Bool()


	// ## Command specific Flags ##
	// Kingpin allows us to specify flags which are only valid for specific sub commands.
	// I.E. the following flag would only be valid for grafana-backup backup
	//ducklings        = backupCMD.Flag("ducklings", "Back up duckings").Bool()

	// A note from the docks
	// Kingpin supports nested sub-commands, with separate flag and positional arguments per sub-command. Note that positional arguments may only occur after sub-commands.

	// Use any of these for argPath
	restorePath      = restoreCMD.Arg("pattern", "A pattern specifying the files to restore.").ExistingFilesOrDirs()
	lsPath           = lsFilesCMD.Arg("pattern", "A pattern specifying the files to view.").ExistingFilesOrDirs()
	confSetPath      = configSetCMD.Arg("pattern", "A pattern specifying the config to restore.").ExistingFilesOrDirs()

	argPath          *[]string

	// A pattern of files to look for if no files list is supplied.
	defaultFilesPattern = "backup/*.json"
)

var cancel = make(chan os.Signal, 1)

// TODO use first $XDG_CONFIG_HOME then try $XDG_CONFIG_DIRS
var tryConfigDirs = []string{"~/.config/grafana+", ".grafana+"}

func main() {
	// TODO parse config here

	kingpin.CommandLine.HelpFlag.Short('h')

	signal.Notify(cancel, os.Interrupt, syscall.SIGTERM)

	//TODO: There has to be a better way to do this than this "argPath = restorePath" that I'm doing below

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case backupCMD.FullCommand():
		// TODO fix logic accordingly with apply-for
		doBackup(serverInstance, applyFor, matchDashboard)
	case restoreCMD.FullCommand():
		argPath = restorePath
		// TODO fix logic accordingly with apply-for
		doRestore(serverInstance, applyFor, matchFilename)
	case lsCMD.FullCommand():
		// TODO fix logic accordingly with apply-for
		doObjectList(serverInstance, applyFor, matchDashboard)
	case lsFilesCMD.FullCommand():
		argPath = lsPath
		// TODO merge this command with ls
		doFileList(matchFilename, applyFor, matchDashboard)
	case configGetCMD.FullCommand():
		// TBD
		// doConfigGet()
		fmt.Fprintln(os.Stderr, "Command config-get not yet implemented!")
	case configSetCMD.FullCommand():
		// TBD
		// doConfigSet()
		argPath = confSetPath
		fmt.Fprintln(os.Stderr, "Command config-set not yet implemented!")
	// default: is not be neccessary with kingpin
	}
}

type command struct {
	grafana             *sdk.Client
	applyHierarchically bool
	applyForBoards      bool
	applyForDs          bool
	applyForUsers       bool
	boardTitle          string
	tags                []string
	starred             bool
	filenames           []string
	force               bool
	verbose             bool
}

type option func(*command) error

func serverInstance(c *command) error {
	c.grafana = sdk.NewClient((*flagServerURL).String(), *flagServerKey, &http.Client{Timeout: *flagTimeout})
	return nil
}

func applyFor(c *command) error {
	if *flagApplyFor == "" {
		return fmt.Errorf("flag '-apply-for' provided with empty argument")
	}
	for _, objectKind := range strings.Split(strings.ToLower(*flagApplyFor), ",") {
		switch objectKind {
		case "auto":
			c.applyHierarchically = true
			c.applyForBoards = true
			c.applyForDs = true
		case "all":
			c.applyForBoards = true
			c.applyForDs = true
			c.applyForUsers = true
		case "dashboards":
			c.applyForBoards = true
		case "datasources":
			c.applyForDs = true
		case "users":
			c.applyForUsers = true
		default:
			return fmt.Errorf("unknown argument %s", objectKind)
		}
	}
	return nil
}

func matchDashboard(c *command) error {
	c.boardTitle = *flagBoardTitle
	c.starred = *flagStarred

	// Check and see if they supplied a comma separated list of tags instead of multiple tag flags.
	if len(*flagTags) == 1 {
		for _, tag := range strings.Split((*flagTags)[0], ",") {
			c.tags = append(c.tags, strings.TrimSpace(tag))
		}
	} else if len(*flagTags) > 1 {
		c.tags = *flagTags
	}

	//if *flagTags != "" {
	//	for _, tag := range strings.Split(*flagTags, ",") {
	//		c.tags = append(c.tags, strings.TrimSpace(tag))
	//	}
	//}
	return nil
}

//Looks in a default location if no files are provided.
func matchFilename(c *command) error {
	if len(*argPath) == 0 {
		files, err := filepath.Glob(defaultFilesPattern);

		if err != nil {
			return err
		}

		if len(files) == 0 {
			return errors.New("No files found in the default location ($CWD/backups/)")
		}

		*argPath = files
		//return errors.New("there are no files matching selected pattern found")
	}

	c.filenames = *argPath
	return nil
}

func initCommand(opts ...option) *command {
	var (
		cmd = &command{force: *flagForce, verbose: *flagVerbose}
		err error
	)
	for _, opt := range opts {
		if err = opt(cmd); err != nil {
			kingpin.Fatalf(fmt.Sprintf("Error: %s\n\n", err))
		}
	}
	return cmd
}
