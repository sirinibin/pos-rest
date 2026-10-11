package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sirinibin/startpos/backend/db"
	"github.com/sirinibin/startpos/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// repairLedgersCommand is the command-line mode of the one-off ledger repair
// (models/ledger_repair.go): it prints the report as JSON and exits without
// starting the server.
//
//	<binary> repair-ledgers [--apply] [--store=<id>]   dry run unless --apply
//	<binary> repair-ledgers --restore [--store=<id>]   undo the applied runs
const repairLedgersCommand = "repair-ledgers"

// parseRepairLedgersArgs parses the arguments after "repair-ledgers".
func parseRepairLedgersArgs(args []string) (storeID *primitive.ObjectID, apply, restore bool, err error) {
	fs := flag.NewFlagSet(repairLedgersCommand, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&apply, "apply", false, "change the data (default: dry run)")
	fs.BoolVar(&restore, "restore", false, "put back what the applied runs changed")
	store := fs.String("store", "", "one store id (default: every store)")
	if err := fs.Parse(args); err != nil {
		return nil, false, false, err
	}
	if fs.NArg() > 0 {
		return nil, false, false, errors.New("unexpected argument " + fs.Arg(0))
	}
	if apply && restore {
		return nil, false, false, errors.New("--apply and --restore are exclusive")
	}
	if *store != "" {
		oid, err := primitive.ObjectIDFromHex(*store)
		if err != nil {
			return nil, false, false, errors.New("invalid --store id")
		}
		storeID = &oid
	}
	return storeID, apply, restore, nil
}

// runRepairLedgers runs the command and returns the process exit code.
func runRepairLedgers(args []string, out, errOut io.Writer) int {
	storeID, apply, restore, err := parseRepairLedgersArgs(args)
	if err != nil {
		fmt.Fprintln(errOut, "usage: "+repairLedgersCommand+" [--apply | --restore] [--store=<id>]: "+err.Error())
		return 2
	}
	db.Client("")
	db.InitRedis()
	var result interface{}
	if restore {
		result, err = models.RunLedgerRepairRestore(storeID)
	} else {
		result, err = models.RunLedgerRepair(storeID, apply)
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
	if err != nil {
		fmt.Fprintln(errOut, "error: "+err.Error())
		return 1
	}
	return 0
}

// repairLedgersMode runs the repair command when the binary was started with
// it; it reports whether it did (then the process exits).
func repairLedgersMode() bool {
	if len(os.Args) < 2 || os.Args[1] != repairLedgersCommand {
		return false
	}
	os.Exit(runRepairLedgers(os.Args[2:], os.Stdout, os.Stderr))
	return true
}
