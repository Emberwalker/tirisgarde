package cmd

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const archiveNameTimeFormat = "2006-01-02_15-04-05.zip"

var Version string = "devel"
var sourcePath string

var rootCmd = &cobra.Command{
	Use:     "tirisgarde",
	Short:   "WoW config backup tool",
	Long:    `Backup WoW WTF directory to a specific location as an archive.`,
	Args:    cobra.NoArgs,
	Version: Version,
	RunE:    run,
	PostRunE: func(cmd *cobra.Command, args []string) error {
		return viper.WriteConfig()
	},
}

func log(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, ">> "+format+"\n", args...)
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&sourcePath, "source", "i", ".", "WoW client directory (usually 'World of Warcraft/_retail_')")
	rootCmd.PersistentFlags().StringP("dest", "o", filepath.Join(".", "WTF-Backup"), "Folder to store backups")
	rootCmd.PersistentFlags().Uint("max-age", 30, "Max age for backups, in days")

	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name != "source" { // Skip the source, since that's always commandline-specified.
			viper.BindPFlag(f.Name, f)
		}
	})
}

func initConfig() {
	sourcePath, err := filepath.Abs(sourcePath)
	cobra.CheckErr(err)

	cfgFile := filepath.Join(sourcePath, ".tirisgarde.yaml")
	viper.SetConfigFile(cfgFile)
	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log("Using config file: %s", viper.ConfigFileUsed())
	}
}

func archive(dest string, basePath string, files []string) error {
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	bar := progressbar.Default(int64(len(files)), filepath.Base(dest))
	defer bar.Close()
	zWrite := zip.NewWriter(f)
	for _, file := range files {
		relPath, err := filepath.Rel(basePath, file)
		if err != nil {
			return nil
		}
		entryWriter, err := zWrite.Create(relPath)
		if err != nil {
			return nil
		}
		contents, err := os.ReadFile(file)
		if err != nil {
			return nil
		}
		_, err = entryWriter.Write(contents)
		if err != nil {
			return nil
		}
		bar.Add(1)
	}
	bar.Finish()

	if err = zWrite.Close(); err != nil {
		return err
	}
	return f.Close()
}

func run(cmd *cobra.Command, args []string) error {
	files := *new([]string)
	wtfDir := filepath.Join(sourcePath, "WTF")
	err := filepath.WalkDir(wtfDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	cobra.CheckErr(err)
	log("%v files to archive", len(files))

	destPath := viper.GetString("dest")
	archiveName := time.Now().Format(archiveNameTimeFormat)
	archivePath := filepath.Join(destPath, archiveName)

	log("Archiving to: %v", archivePath)
	cobra.CheckErr(os.MkdirAll(destPath, os.ModeDir))
	cobra.CheckErr(archive(archivePath, wtfDir, files))
	log("Archive completed: %v", archivePath)

	maxDays := viper.GetUint("max-age")
	maxHrs := maxDays * 24
	log("Pruning backups older than %v days...", maxDays)
	delta, _ := time.ParseDuration(fmt.Sprintf("%vh", maxHrs))
	cutoff := time.Now().Add(-delta)
	zips, err := filepath.Glob(filepath.Join(destPath, "*.zip"))
	cobra.CheckErr(err)
	for _, path := range zips {
		ts, err := time.ParseInLocation(archiveNameTimeFormat, filepath.Base(path), time.Local)
		if err != nil {
			continue // Skip non-matching files
		}
		if ts.Before(cutoff) {
			log("Pruning: %s", path)
			os.Remove(path)
		}
	}

	return nil
}
