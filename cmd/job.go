package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tta-lab/einai/internal/jobqueue"
)

func init() {
	rootCmd.AddCommand(jobCmd)
}

var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "Manage background jobs",
}

var jobListCmd = &cobra.Command{
	Use:   "list",
	Short: "List background jobs",
	RunE:  runJobList,
}

var jobLogCmd = &cobra.Command{
	Use:   "log",
	Short: "Print job output",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobLog,
}

var jobKillCmd = &cobra.Command{
	Use:   "kill",
	Short: "Kill a running or queued job",
	Args:  cobra.ExactArgs(1),
	RunE:  runJobKill,
}

var jobListLimit int
var jobListJSON bool

func init() {
	jobListCmd.Flags().IntVar(&jobListLimit, "limit", 20, "Maximum number of jobs to list")
	jobListCmd.Flags().BoolVar(&jobListJSON, "json", false, "Output as JSON")

	jobCmd.AddCommand(jobListCmd, jobLogCmd, jobKillCmd)
}

func runJobList(cmd *cobra.Command, args []string) error {
	client := newUnixClient()
	u := "http://unix/job/list?limit=" + strconv.Itoa(jobListLimit)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req.WithContext(cmd.Context()))
	if err != nil {
		return fmt.Errorf("daemon request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, body)
	}

	if jobListJSON {
		fmt.Print(string(body))
		return nil
	}

	var result struct {
		Jobs []jobqueue.Job `json:"jobs"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\tSTATE\tAGENT\tRUNTIME\tSTARTED\tSEND_TARGET\n")
	for _, j := range result.Jobs {
		started := formatTime(j.StartedAt)
		st := j.SendTarget
		if st == "" {
			st = "-"
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", j.ID, j.State, j.Agent, j.Runtime, started, st)
	}
	return tw.Flush()
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Local().Format("15:04:05")
}

func runJobLog(cmd *cobra.Command, args []string) error {
	id := args[0]
	client := newUnixClient()
	u := "http://unix/job/log?id=" + url.QueryEscape(id)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req.WithContext(cmd.Context()))
	if err != nil {
		return fmt.Errorf("daemon request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("job %s not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, body)
	}

	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}

func runJobKill(cmd *cobra.Command, args []string) error {
	id := args[0]
	client := newUnixClient()
	u := "http://unix/job/kill?id=" + url.QueryEscape(id)
	resp, err := client.Post(u, "", nil)
	if err != nil {
		return fmt.Errorf("daemon request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon error (%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if !result.Ok {
		return fmt.Errorf("%s", result.Error)
	}
	fmt.Println("killed")
	return nil
}
