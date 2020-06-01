package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/MakeNowJust/heredoc"
	"github.com/mhristof/germ/aws"
	"github.com/mhristof/germ/iterm"
	"github.com/spf13/cobra"
)

var command string

var cmdCmd = &cobra.Command{
	Use:   "cmd",
	Short: "Generate the bash code required to run a command accross the whole AWS estate",
	Long: heredoc.Doc(
		`Command variables are:
		    {{ .Profile }} will be replaced with the current profile
			{{ .Region }} If this is present, the command will be executed in all AWS regions. Warning, this is whitespace sensitive
		`,
	),
	Run: func(cmd *cobra.Command, args []string) {
		Verbose(cmd)

		var prof = iterm.Profiles{
			Profiles: aws.Profiles(AWSConfig),
		}

		fmt.Println(generateCommands(prof, command))
	},
}

func generateCommands(prof iterm.Profiles, command string) []string {
	var ret []string

	for source, profiles := range prof.ProfileTree() {
		login := false
		for _, profile := range profiles {
			if !login {
				loginGUID := fmt.Sprintf("login-%s", source)
				iProfile, found := prof.FindGUID(loginGUID)
				if !found {
					panic(loginGUID)
				}

				ret = append(ret, strings.Replace(iProfile.Command, " || sleep 60'", "'", -1))
				login = true
			}

			tCommand := fmt.Sprintf("AWS_PROFILE={{ .Profile }} %s", command)
			str := generateTemplate(tCommand, profile)
			ret = append(ret, str)
		}
	}
	return ret
}

func generateTemplate(command, profile string) string {
	t, err := template.New(profile).Parse(command)
	if err != nil {
		panic(err)
	}

	var tpl bytes.Buffer
	if strings.Contains(command, "{{ .Region }}") {
		for _, region := range aws.Regions() {
			err = t.Execute(&tpl, struct {
				Profile string
				Region  string
			}{
				Profile: profile,
				Region:  region,
			})
		}
	} else {
		err = t.Execute(&tpl, struct {
			Profile string
		}{
			Profile: profile,
		})
	}

	return tpl.String()
}

func init() {
	cmdCmd.Flags().StringVarP(&command, "cmd", "", "aws s3 ls", "command to run")

	rootCmd.AddCommand(cmdCmd)
}
