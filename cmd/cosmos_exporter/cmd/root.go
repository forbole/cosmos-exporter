package cmd

import (
	"fmt"
	"os"
	"path"

	Config "github.com/forbole/cosmos-exporter/types/config"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	homeDir string
	config  *Config.Config
)

var rootCmd = &cobra.Command{
	Use:   "cosmos_exporter",
	Short: "A cosmos exporter to export validator and delegator balances",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	handleInitError(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&homeDir, "home", "", "Directory for config and data (default is $HOME/.cosmos_exporter)")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if homeDir != "" {
		cfgFile := path.Join(homeDir, "config.yaml")
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := homedir.Dir()
		handleInitError(err)
		viper.AddConfigPath(path.Join(home, ".cosmos_exporter"))
		viper.SetConfigName("config")
	}
}

func handleInitError(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
