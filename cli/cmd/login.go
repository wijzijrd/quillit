package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/quillit/cli/internal/client"
)

var loginServer string

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to a Quillit web app, storing the session for `quillit push`.",
	Long: `Authenticate against a Quillit web app and store the session in the
home config. quillit push uses it to import projects. Sessions last as
long as browser sessions; log in again when one expires.`,
	Example: `  quillit login --server https://quillit.local`,
	RunE: func(cmd *cobra.Command, args []string) error {
		h, err := requireHome()
		if err != nil {
			return err
		}

		server := loginServer
		if server == "" {
			server = h.Config.Server
		}
		if server == "" {
			return fmt.Errorf("no server known — pass --server, e.g. `quillit login --server https://quillit.local`")
		}

		fmt.Print("Email: ")
		reader := bufio.NewReader(os.Stdin)
		email, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		email = strings.TrimSpace(email)

		fmt.Print("Password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}

		token, err := client.Login(server, email, string(pw))
		if err != nil {
			return err
		}

		h.Config.Server = server
		h.Config.SessionToken = token
		if err := h.Save(); err != nil {
			return err
		}
		fmt.Printf("Logged in to %s\n", server)
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Forget the stored web app session.",
	RunE: func(cmd *cobra.Command, args []string) error {
		h, err := requireHome()
		if err != nil {
			return err
		}
		h.Config.SessionToken = ""
		if err := h.Save(); err != nil {
			return err
		}
		fmt.Println("Logged out")
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginServer, "server", "", "Web app base URL, e.g. https://quillit.local")
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}
