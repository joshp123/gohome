package main

import (
	"context"
	"fmt"

	"github.com/joshp123/gohome/internal/render"
	tadov1 "github.com/joshp123/gohome/proto/gen/plugins/tado/v1"
	"google.golang.org/grpc"
)

func tadoCmd(ctx context.Context, conn *grpc.ClientConn, out render.Renderer, args []string) int {
	sub := "zones"
	if len(args) > 0 {
		sub = args[0]
	}

	client := tadov1.NewTadoServiceClient(conn)
	switch sub {
	case "zones", "list":
		resp, err := client.ListZones(ctx, &tadov1.ListZonesRequest{})
		if err != nil {
			out.WriteError("tado zones", err, render.ExitRuntimeError)
			return render.ExitRuntimeError
		}
		if out.Format == render.FormatJSON {
			_ = out.WriteProto(resp)
			return render.ExitOK
		}
		rows := make([][]string, 0, len(resp.Zones))
		for _, zone := range resp.Zones {
			rows = append(rows, []string{zone.GetName(), zone.GetId()})
		}
		_ = out.WriteTable([]string{"ZONE", "ID"}, rows)
		return render.ExitOK
	case "set":
		out.WriteError("tado set", fmt.Errorf("gohome-cli is read-only; use 'gohome-act tado set <zone> <temp_c>' for policy-gated writes"), render.ExitUsageError)
		return render.ExitUsageError
	default:
		out.WriteHelp("gohome-cli tado <zones>", nil)
		return render.ExitUsageError
	}
}
