package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/adrg/xdg"
	"github.com/google/go-cmp/cmp"
	"github.com/mhristof/germ/aws"
	"github.com/mhristof/germ/config"
	"github.com/mhristof/germ/iterm"
	"github.com/mhristof/germ/k8s"
	"github.com/mhristof/germ/ssh"
	"github.com/mhristof/germ/ssm"
	"github.com/mhristof/germ/vault"
	"github.com/mhristof/germ/vim"
	"github.com/rs/zerolog/log"

	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
)

var (
	output          string
	write           bool
	kubeConfig      string
	diff            bool
	ignoreInstances bool
	AWSConfig       = expandUser("~/.aws/config")
	AWSCredentials  = expandUser("~/.aws/credentials")
	DefaultProfile  = "default-profile"
)

var generateCmd = &cobra.Command{
	Use:     "generate",
	Short:   "Generate the profiles",
	Aliases: []string{"gen"},
	Run: func(cmd *cobra.Command, args []string) {
		Verbose(cmd)

		if write && dryRun {
			log.Fatal().Msg("--write is incompatible with --dry-run")
		}

		if write && diff {
			log.Fatal().Msg("--write and --diff are incompatible")
		}

		var prof iterm.Profiles

		prof.Profiles = append(prof.Profiles, aws.Profiles("", AWSConfig)...)
		prof.Profiles = append(prof.Profiles, k8s.Profiles(kubeConfig, dryRun)...)
		prof.Profiles = append(prof.Profiles, keyChain.Profiles()...)
		prof.Profiles = append(prof.Profiles, *iterm.NewProfile(DefaultProfile, map[string]string{
			"AllowTitleSetting": "true",
			"BadgeText":         "",
		}))
		prof.Profiles = append(prof.Profiles, vim.Profile())
		prof.Profiles = append(prof.Profiles, ssh.Profiles()...)

		config.Load()
		prof.Profiles = append(prof.Profiles, config.Generate()...)
		if !ignoreInstances {
			ssmProfs := ssm.Generate()
			prof.Profiles = append(prof.Profiles, ssmProfs...)

			data, err := json.MarshalIndent(ssmProfs, "", "    ")
			if err != nil {
				log.Fatal().Err(err).Msg("cannot marshal ssm profiles")
			}

			storeToCache("germ.ssm.json", data)
		} else {
			data, path := loadFromCache("germ.ssm.json")

			var ssmProfs []iterm.Profile
			err := json.Unmarshal(data, &ssmProfs)
			if err != nil {
				log.Fatal().Str("path", path).Err(err).Msg("cannot unmarshal ssm profiles")
			} else {
				prof.Profiles = append(prof.Profiles, ssmProfs...)
				log.Info().Str("path", path).Msg("using cached ssm profiles")
			}
		}

		vaultProfile, err := vault.Profile()
		if err != nil {
			log.Warn().Err(err).Msg("cannot add vault profile")
		} else {
			prof.Profiles = append(prof.Profiles, vaultProfile)
		}

		prof.UpdateKeyboardMaps()
		prof.UpdateAWSSmartSelectionRules()

		var uniqProf iterm.Profiles
		existingProfiles := map[string]struct{}{}
		for _, profile := range prof.Profiles {
			if _, ok := existingProfiles[profile.Name]; ok {
				log.Warn().Str("name", profile.Name).Msg("duplicate profile")
				continue
			}

			existingProfiles[profile.Name] = struct{}{}
			uniqProf.Profiles = append(uniqProf.Profiles, profile)

		}

		prof = uniqProf

		profJSON, err := json.MarshalIndent(prof, "", "    ")
		if err != nil {
			log.Fatal().Err(err).Msg("cannot indent json")
		}

		// unescape "&" character.
		profJSON = []byte(strings.ReplaceAll(string(profJSON), `\u0026`, "&"))
		// unescape ">" character.
		profJSON = []byte(strings.ReplaceAll(string(profJSON), `\u003e`, ">"))

		if write {
			err = ioutil.WriteFile(output, profJSON, 0o644)
			if err != nil {
				log.Fatal().Err(err).Msg("cannot write to file")
			}
		} else if diff {
			curr, err := ioutil.ReadFile(output)
			if err != nil {
				log.Fatal().Err(err).Msg("cannot read file")
			}

			var current iterm.Profiles
			err = json.Unmarshal(curr, &current)
			if err != nil {
				log.Fatal().Err(err).Msg("cannot unmarshal output file")
			}

			sort.Slice(current.Profiles, func(i, j int) bool {
				return current.Profiles[i].GUID < current.Profiles[j].GUID
			})

			sort.Slice(prof.Profiles, func(i, j int) bool {
				return prof.Profiles[i].GUID < prof.Profiles[j].GUID
			})

			if diff := cmp.Diff(current, prof); diff != "" {
				fmt.Println("Updating (-current +new):", diff)
			}
		} else {
			fmt.Println(string(profJSON))
		}
	},
}

func loadFromCache(name string) ([]byte, string) {
	path, err := xdg.CacheFile(name)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot get cache file")
		return nil, path
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot read from file")
		return nil, path
	}

	log.Debug().Str("path", path).Str("name", name).Msg("loaded from cache")
	return data, path
}

func storeToCache(name string, data []byte) {
	path, err := xdg.CacheFile(name)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot get cache file")
	}

	// write to file
	err = ioutil.WriteFile(path, data, 0o644)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot write to file")
	}

	log.Debug().Str("path", path).Str("name", name).Msg("stored to cache")
}

func expandUser(path string) string {
	out, err := homedir.Expand(path)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot expand homedir")
	}
	return out
}

func init() {
	generateCmd.Flags().StringVarP(
		&output, "output", "o",
		expandUser("~/Library/Application Support/iTerm2/DynamicProfiles/aws-profiles.json"),
		"File to save the generated profiles",
	)
	generateCmd.Flags().StringVarP(
		&AWSConfig, "aws-config", "a",
		AWSConfig,
		"AWS config file path",
	)
	generateCmd.Flags().StringVarP(
		&AWSCredentials, "aws-credentials", "c",
		AWSCredentials,
		"AWS credentials file path",
	)
	generateCmd.Flags().StringVarP(
		&kubeConfig, "kube-config", "k",
		expandUser("~/.kube/config"),
		"Kubernetes configuration file",
	)
	generateCmd.Flags().BoolVarP(&write, "write", "w", false, "Write the output to the destination file")
	generateCmd.Flags().BoolVarP(&diff, "diff", "d", false, "Generate a diff for the new changes")
	generateCmd.Flags().BoolVarP(&ignoreInstances, "ignore-instances", "I", false, "Ignore SSM instance profiles")

	rootCmd.AddCommand(generateCmd)
}
