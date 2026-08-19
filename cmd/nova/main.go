package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/luaxlou/glow-ops/internal/client"
)

func usage() {
	fmt.Println(`nova (Nova Run CLI)

Usage:
  nova deploy <app> <artifact_dir>
  nova start <app>
  nova stop <app>
  nova restart <app>
  nova status <app>
  nova logs <app> [-f]
  nova list
  nova remove <app>

Local convenience:
  nova rollback <app>
  nova target add <name> --url <url> --token <token>
  nova target use <name>
  nova target list
`)
}

func main() {
	args := os.Args
	if len(args) < 2 {
		usage()
		os.Exit(1)
	}

	cli := client.NewClient()
	ctx := context.Background()
	cmd := args[1]
	rest := args[2:]

	switch cmd {
	case "deploy":
		if len(rest) < 2 {
			fmt.Println("ERROR: nova deploy <app> <artifact_dir>")
			os.Exit(1)
		}
		if err := cli.Deploy(ctx, rest[0], rest[1]); err != nil {
			fmt.Printf("deploy failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("deployed")

	case "start":
		ensureName(rest)
		if err := cli.Start(ctx, rest[0]); err != nil {
			fmt.Printf("start failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("start command sent")
	case "stop":
		ensureName(rest)
		if err := cli.Stop(ctx, rest[0]); err != nil {
			fmt.Printf("stop failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("stop command sent")
	case "restart":
		ensureName(rest)
		if err := cli.Restart(ctx, rest[0]); err != nil {
			fmt.Printf("restart failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("restart command sent")
	case "status":
		ensureName(rest)
		status, err := cli.Status(ctx, rest[0])
		if err != nil {
			fmt.Printf("status failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(status)
	case "logs":
		ensureName(rest)
		follow := len(rest) > 1 && strings.EqualFold(rest[1], "-f")
		if follow {
			if err := cli.LogsStream(ctx, rest[0], os.Stdout); err != nil {
				fmt.Printf("logs failed: %v\n", err)
				os.Exit(1)
			}
		} else {
			stream, err := cli.Logs(ctx, rest[0], follow)
			if err != nil {
				fmt.Printf("logs failed: %v\n", err)
				os.Exit(1)
			}
			for _, line := range stream {
				fmt.Println(line)
			}
		}
	case "list":
		list, err := cli.List(ctx)
		if err != nil {
			fmt.Printf("list failed: %v\n", err)
			os.Exit(1)
		}
		for _, item := range list {
			fmt.Println(item)
		}
	case "remove":
		ensureName(rest)
		if err := cli.Remove(ctx, rest[0]); err != nil {
			fmt.Printf("remove failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("removed")
	case "rollback":
		ensureName(rest)
		fmt.Println("rollback is a local operation; not implemented in this milestone")
	case "target":
		if len(rest) == 0 {
			usage()
			os.Exit(1)
		}
		switch rest[0] {
		case "add":
			fmt.Println("target add: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		case "use":
			fmt.Println("target use: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		case "list":
			fmt.Println("target list: use nova config file at ~/.nova/contexts (not implemented in this milestone)")
		default:
			usage()
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func ensureName(rest []string) {
	if len(rest) < 1 || strings.TrimSpace(rest[0]) == "" {
		fmt.Println("ERROR: missing app name")
		os.Exit(1)
	}
}
