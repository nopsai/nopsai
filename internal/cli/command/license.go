package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

const proprietaryLicenseNotice = `NopsAI Proprietary Software Notice

Copyright (c) 2026 Hossein Yousefi. All rights reserved.

NopsAI is proprietary software. Possession of or access to this binary does not
grant a licence to use it. Use is permitted only under a written agreement
signed by Hossein Yousefi or by a successor entity to which the relevant rights
have been assigned.

Third-party components remain subject to their applicable licence terms. See the
LICENSE and THIRD_PARTY_NOTICES.md files supplied with the NopsAI release.

Licensing enquiries: contact@nopsai.com`

func newLicenseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "license",
		Short: "Show the NopsAI proprietary software notice",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(command.OutOrStdout(), proprietaryLicenseNotice)
			return err
		},
	}
}
