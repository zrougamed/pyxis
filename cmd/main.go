// Package main provides the entry point for the pyxis CLI.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/zrougamed/pyxis/internal/k8s"
	"github.com/zrougamed/pyxis/internal/tui"
	"github.com/zrougamed/pyxis/internal/webapp"
)

var (
	version    = "dev"
	kubeconfig string
	context_   string
	namespace  string
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "pyxis",
		Short:   "⎈ Pyxis — the operator compass for Kubernetes",
		Long:    `A portable, interactive TUI for daily Kubernetes operations. Fuzzy search pods, tail logs, inspect images and env vars, switch contexts, and more.`,
		Version: version,
		RunE:    runTUI,
	}

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (default: ~/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&context_, "context", "", "Kubernetes context to use")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "Kubernetes namespace (empty = all namespaces)")

	// Subcommands for non-interactive use
	rootCmd.AddCommand(logsCmd())
	rootCmd.AddCommand(imagesCmd())
	rootCmd.AddCommand(envCmd())
	rootCmd.AddCommand(contextsCmd())
	rootCmd.AddCommand(podsCmd())
	rootCmd.AddCommand(overviewCmd())
	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(webCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	client, err := k8s.NewClient(kubeconfig, context_)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}

	model := tui.NewModel(client, namespace)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	return nil
}

func runTUIView(view tui.View, search string, tail int64) error {
	client, err := k8s.NewClient(kubeconfig, context_)
	if err != nil {
		return fmt.Errorf("connecting to cluster: %w", err)
	}
	model := tui.NewModelWithOptions(client, namespace, tui.LaunchOptions{
		InitialView: view,
		Search:      search,
		TailLines:   tail,
	})
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}
	return nil
}

func logsCmd() *cobra.Command {
	var tailLines int64
	cmd := &cobra.Command{
		Use:   "logs [pod-name-pattern]",
		Short: "Open the TUI on pod logs (optional fuzzy pattern)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := ""
			if len(args) > 0 {
				pattern = args[0]
			}
			return runTUIView(tui.ViewPodLogs, pattern, tailLines)
		},
	}
	cmd.Flags().Int64Var(&tailLines, "tail", 100, "Number of log lines to show")
	return cmd
}

func imagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "images",
		Short: "List container images from pods",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				return err
			}

			pods, err := client.ListPods(cmd.Context(), namespace, k8s.PodFilterAll)
			if err != nil {
				return err
			}

			seen := make(map[string]bool)
			for _, pod := range pods {
				for _, img := range pod.Images {
					if !seen[img] {
						fmt.Printf("%-60s %s/%s (%s)\n", img, pod.Namespace, pod.Name, pod.Phase)
						seen[img] = true
					}
				}
			}
			return nil
		},
	}
}

func envCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env [pod-name]",
		Short: "Get pod environment variables",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				return err
			}

			ns := namespace
			if ns == "" {
				ns = "default"
			}

			containers, err := client.GetPodEnvVars(cmd.Context(), ns, args[0])
			if err != nil {
				return err
			}

			for _, c := range containers {
				fmt.Printf("# Container: %s (%s)\n", c.Name, c.Image)
				for _, env := range c.EnvVars {
					if env.Value != "" {
						fmt.Printf("%s=%s\n", env.Name, env.Value)
					} else {
						fmt.Printf("%s=%s\n", env.Name, env.ValueFrom)
					}
				}
				fmt.Println()
			}
			return nil
		},
	}
}

func contextsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "contexts",
		Short: "List available kubeconfig contexts",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				return err
			}

			current := client.CurrentContext()
			for _, ctx := range client.Contexts() {
				marker := "  "
				if ctx == current {
					marker = "* "
				}
				fmt.Printf("%s%s\n", marker, ctx)
			}
			return nil
		},
	}
}

func podsCmd() *cobra.Command {
	var openTUI bool
	cmd := &cobra.Command{
		Use:   "pods [name-pattern]",
		Short: "List pods with status (or open TUI with --tui)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pattern := ""
			if len(args) > 0 {
				pattern = args[0]
			}
			if openTUI {
				return runTUIView(tui.ViewPodList, pattern, 0)
			}

			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				return err
			}

			pods, err := client.ListPods(cmd.Context(), namespace, k8s.PodFilterAll)
			if err != nil {
				return err
			}

			fmt.Printf("%-50s %-12s %-8s %-10s %s\n", "NAME", "NAMESPACE", "STATUS", "READY", "RESTARTS")
			for _, p := range pods {
				if pattern != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(pattern)) {
					continue
				}
				fmt.Printf("%-50s %-12s %-8s %-10s %d\n",
					p.Name, p.Namespace, p.Phase, p.Ready, p.Restarts)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&openTUI, "tui", false, "Open the interactive TUI pods view")
	return cmd
}

func overviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "overview",
		Short: "Open the TUI cluster overview",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUIView(tui.ViewOverview, "", 0)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version info",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("pyxis %s\n", version)

			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				fmt.Printf("Kubernetes server: unable to connect\n")
				return nil
			}

			sv, err := client.ServerVersion()
			if err != nil {
				fmt.Printf("Kubernetes server: unable to get version\n")
				return nil
			}
			fmt.Printf("Kubernetes server: %s\n", sv)
			return nil
		},
	}
}

func webCmd() *cobra.Command {
	var cfg webapp.Config
	var dexScopes string

	cmd := &cobra.Command{
		Use:   "web",
		Short: "Run the responsive React web app and JSON API",
		Long:  "Serve a responsive React dashboard with a JSON API for desktop and mobile browsers. Use --no-auth for local unauthenticated access, or configure Dex OIDC.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := k8s.NewClient(kubeconfig, context_)
			if err != nil {
				return fmt.Errorf("connecting to cluster: %w", err)
			}
			cfg.Scopes = splitAndTrim(dexScopes)
			server, err := webapp.NewServer(cfg, client)
			if err != nil {
				return err
			}
			mode := "auth"
			if cfg.NoAuth {
				mode = "no-auth"
			}
			fmt.Printf("Starting pyxis web on %s (%s)\n", cfg.ListenAddr, mode)
			return server.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&cfg.ListenAddr, "listen", ":8080", "HTTP listen address for the web app")
	cmd.Flags().StringVar(&cfg.BaseURL, "base-url", "http://localhost:8080", "External base URL used for Dex callback redirects")
	cmd.Flags().StringVar(&cfg.CookieSecret, "cookie-secret", "", "Secret used to sign the web session cookie (required unless --no-auth)")
	cmd.Flags().BoolVar(&cfg.NoAuth, "no-auth", false, "Disable authentication (local/dev only; do not expose publicly)")
	cmd.Flags().StringVar(&cfg.DexIssuer, "dex-issuer", "", "Dex issuer URL (for example https://dex.example.com)")
	cmd.Flags().StringVar(&cfg.DexClientID, "dex-client-id", "", "Dex OAuth client ID")
	cmd.Flags().StringVar(&cfg.DexClientSecret, "dex-client-secret", "", "Dex OAuth client secret")
	cmd.Flags().StringVar(&cfg.DexPublicIssuer, "dex-public-issuer", "", "Public Dex issuer/base URL for browser redirects when the backend reaches Dex on an internal address")
	cmd.Flags().StringVar(&dexScopes, "dex-scopes", "openid,profile,email", "Comma-separated Dex scopes to request")

	return cmd
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
