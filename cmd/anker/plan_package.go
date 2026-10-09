package main

import (
	"anker/internal/anker"
	"encoding/json"
	"errors"
	"flag"
	"os"
)

func runPlanPackage(args []string) error {
	if len(args) == 0 || args[0] != "verify" {
		return errors.New("Offline-Prüfung: anker plan-package verify --input PLAN.tar")
	}
	f := flag.NewFlagSet("plan-package verify", flag.ContinueOnError)
	input := f.String("input", "", "Planpaket (tar), wird ausschließlich gelesen")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if *input == "" || f.NArg() != 0 {
		return errors.New("Offline-Prüfung benötigt genau --input PLAN.tar")
	}
	file, err := os.Open(*input)
	if err != nil {
		return err
	}
	defer file.Close()
	st, err := file.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("Planpaket muss eine gewöhnliche Datei sein")
	}
	result, err := anker.VerifyPlanPackage(file)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
