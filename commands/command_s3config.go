package commands

import (
  "github.com/github/git-lfs/git"
	"github.com/github/git-lfs/vendor/_nuts/github.com/spf13/cobra"
)

var (
	s3ConfigCmd = &cobra.Command{
		Use: "s3config",
		Run: s3ConfigCommand,
	}
)

func s3ConfigCommand(cmd *cobra.Command, args []string) {
  if len(args) != 4 {
		Print("Usage: git lfs s3config <bucket> <region> <accesskey> <secretkey>")
    return
  }

  bucket := args[0]
  region := args[1]
  accesskey := args[2]
  secretkey := args[3]

  git.Config.SetGlobal("filter.lfs.s3.bucket", bucket)
  git.Config.SetGlobal("filter.lfs.s3.region", region)
  git.Config.SetGlobal("filter.lfs.s3.accesskey", accesskey)
  git.Config.SetGlobal("filter.lfs.s3.secretkey", secretkey)

  Print("filter.lfs.s3.bucket=%v", bucket)
  Print("filter.lfs.s3.region=%v", region)
  Print("filter.lfs.s3.accesskey=%v", accesskey)
  Print("filter.lfs.s3.secretkey=%v", secretkey)
}

func init() {
	RootCmd.AddCommand(s3ConfigCmd)
}
