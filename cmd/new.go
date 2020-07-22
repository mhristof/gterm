package cmd

import (
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mhristof/germ/keychain"
	"github.com/mhristof/germ/log"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	newName  string
	value    string
	file     string
	keyChain = keychain.KeyChain{
		Service:     "germ",
		AccessGroup: "germ",
	}
	exported bool
)

var newCmd = &cobra.Command{
	Use:     "new",
	Short:   "Create new profile for the given secret. The system will be entered via a prompt to avoid storing it in the cmd history",
	Aliases: []string{"add"},
	Run: func(cmd *cobra.Command, args []string) {
		Verbose(cmd)

		keyChain.Add(newName, findPassword(file))
	},
}

func findPassword(file string) string {
	if file != "" {
		return handleFile(file)
	}

	fmt.Print("Enter secret:")
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatal("Cannot read secret")
	}

	if exported {
		bytePassword = []byte(fmt.Sprintf("export %s='%s'", strings.ToUpper(newName), string(bytePassword)))
	}

	return string(bytePassword)
}

func handleFile(file string) string {
	base := filepath.Base(file)
	if strings.HasPrefix(base, "credentials") && strings.HasSuffix(file, "csv") {
		records := slurpCsv(file)

		if records[0][2] != "Access key ID" {
			log.WithFields(log.Fields{
				"records[0][2]": records[0][2],
				"file":          file,
			}).Fatal("Invalid header for AWS creds file")
		}

		if records[0][3] != "Secret access key" {
			log.WithFields(log.Fields{
				"records[0][3]": records[0][3],
				"file":          file,
			}).Fatal("Invalid header for AWS creds file")
		}

		return exportAWS(records[1][2], records[1][3])
	}

	if strings.HasPrefix(base, "accessKeys") && strings.HasSuffix(file, "csv") {
		records := slurpCsv(file)

		if records[0][0] != "Access key ID" {
			log.WithFields(log.Fields{
				"records[0][0]": records[0][0],
				"file":          file,
			}).Fatal("Invalid header for AWS creds file")
		}

		if records[0][1] != "Secret access key" {
			log.WithFields(log.Fields{
				"records[0][1]": records[0][1],
				"file":          file,
			}).Fatal("Invalid header for AWS creds file")

		}

		return exportAWS(records[1][0], records[1][1])
	}
	log.WithFields(log.Fields{
		"file": file,
	}).Fatal("Cannot handle this creds file")
	return ""
}

func slurpCsv(file string) [][]string {

	in, err := ioutil.ReadFile(file)
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"file": file,
		}).Fatal("Cannot read file")

	}
	r := csv.NewReader(strings.NewReader(string(in)))

	records, err := r.ReadAll()
	if err != nil {
		log.WithFields(log.Fields{
			"err":  err,
			"file": file,
		}).Fatal("Cannot read CSV file")

	}

	return records
}

func exportAWS(access, secret string) string {
	return fmt.Sprintf("export AWS_ACCESS_KEY_ID=%s AWS_SECRET_ACCESS_KEY=%s", access, secret)
}

func init() {
	newCmd.Flags().StringVarP(&newName, "name", "", "", "Name of the profile")
	newCmd.Flags().StringVarP(&file, "file", "f", "", "Credentials file to parse")
	newCmd.Flags().BoolVarP(&exported, "export", "e", false, "Treat the password as an exported variable. The name of the variable will be the uppercased name provided.")
	newCmd.MarkFlagRequired("name")

	rootCmd.AddCommand(newCmd)
}
