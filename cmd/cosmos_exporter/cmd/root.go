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
	version = "dev" // Set at build time via -ldflags
)

var rootCmd = &cobra.Command{
	Use:   "cosmos_exporter",
	Short: "A cosmos exporter to export validator and delegator balances",
	Run: func(cmd *cobra.Command, args []string) {
		if v, _ := cmd.Flags().GetBool("version"); v {
			fmt.Printf("cosmos_exporter version %s\n", version)
			os.Exit(0)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	handleInitError(rootCmd.Execute())
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&homeDir, "home", "", "Directory for config and data (default is $HOME/.cosmos_exporter)")
	rootCmd.PersistentFlags().String("chain-name", "", "Optional chain-registry chain name for fallback RPC/GRPC endpoints")
	rootCmd.Flags().BoolP("version", "v", false, "Print the version number and exit")
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
