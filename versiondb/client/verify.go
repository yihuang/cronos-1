package client

import (
	"fmt"
	"io"
	"os"

	"cosmossdk.io/errors"
	"github.com/cosmos/iavl"
	"github.com/spf13/cobra"

	"github.com/crypto-org-chain/cronos/versiondb/memiavl"
)

func VerifyPlainFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify-plain [plain-file] [plain-file] ...",
		Short: "Verify change set file by rebuild iavl tree in memory and check root hash, the plain files must include continuous blocks",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetVersion, err := cmd.Flags().GetInt64(flagTargetVersion)
			if err != nil {
				return err
			}
			saveSnapshot, err := cmd.Flags().GetString(flagSaveSnapshot)
			if err != nil {
				return err
			}

			loadSnapshot, err := cmd.Flags().GetString(flagLoadSnapshot)
			if err != nil {
				return err
			}

			if len(saveSnapshot) > 0 {
				// detect the write permission early on.
				if err := os.MkdirAll(saveSnapshot, os.ModePerm); err != nil {
					return err
				}
			}

			var (
				tree  *memiavl.Tree
				blobs *memiavl.PersistedBlobs
			)
			if len(loadSnapshot) > 0 {
				tree, blobs, err = memiavl.LoadTreeFromSnapshot(loadSnapshot)
				if err != nil {
					return errors.Wrapf(err, "fail to load snapshot: %s", loadSnapshot)
				}
				if blobs != nil {
					defer blobs.Close()
				}
				fmt.Printf("snapshot loaded: %d %X\n", tree.Version(), tree.RootHash())
			} else {
				tree = memiavl.NewEmptyTree(0)
			}

			for _, fileName := range args {
				if err := withPlainInput(fileName, func(reader io.Reader) error {
					offset, err := readPlainFile(reader, func(version int64, changeSet *iavl.ChangeSet) (bool, error) {
						if version <= tree.Version() {
							// skip old change sets
							return true, nil
						}

						for _, pair := range changeSet.Pairs {
							if pair.Delete {
								tree.Remove(pair.Key)
							} else {
								tree.Set(pair.Key, pair.Value)
							}
						}

						// no need to update hashes for intermidiate versions.
						_, v, err := tree.SaveVersion(false)
						if err != nil {
							return false, err
						}
						if v != version {
							return false, fmt.Errorf("version don't match: %d != %d", v, version)
						}
						return targetVersion == 0 || v < targetVersion, nil
					}, true)

					if err == io.ErrUnexpectedEOF {
						// incomplete end of file, we'll output a warning and process the completed versions.
						fmt.Fprintf(os.Stderr, "file incomplete, the completed versions are processed, the last completed file offset: %d\n", offset)
					}

					return err
				}); err != nil {
					return err
				}

				if targetVersion > 0 && tree.Version() >= targetVersion {
					break
				}
			}

			rootHash := tree.RootHash()
			fmt.Printf("%d %X\n", tree.Version(), rootHash)

			if len(saveSnapshot) > 0 {
				tree.WriteSnapshot(saveSnapshot)
			}
			return nil
		},
	}
	cmd.Flags().Int64(flagTargetVersion, 0, "specify the target version, otherwise it'll exhaust the plain files")
	cmd.Flags().String(flagSaveSnapshot, "", "save the snapshot of the target iavl tree")
	cmd.Flags().String(flagLoadSnapshot, "", "load the snapshot before doing verification")
	return cmd
}
