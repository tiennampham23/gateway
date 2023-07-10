package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		UseShortOptionHandling: true,
		Name:                   "bxcli",
		Usage:                  "interact with bloxroute gateway",
		Commands: []*cli.Command{
			{
				Name:  "newTxs",
				Usage: "provides a stream of new txs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "filters",
						Required: false,
					},
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdNewTXs,
			},
			{
				Name:  "pendingTxs",
				Usage: "provides a stream of pending txs",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "filters",
						Required: false,
					},
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdPendingTXs,
			},
			{
				Name:  "newBlocks",
				Usage: "provides a stream of new blocks",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdNewBlocks,
			},
			{
				Name:  "bdnBlocks",
				Usage: "provides a stream of new blocks",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "include",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdBdnBlocks,
			},
			{
				Name:  "blxrtx",
				Usage: "send paid transaction",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "transaction",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdBlxrTX,
			},
			{
				Name:  "blxr-batch-tx",
				Usage: "send multiple paid transactions",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "transactions",
						Required: true,
					},
					&cli.BoolFlag{
						Name: "nonce-monitoring",
					},
					&cli.BoolFlag{
						Name: "next-validator",
					},
					&cli.BoolFlag{
						Name: "validators-only",
					},
					&cli.IntFlag{
						Name: "fallback",
					},
					&cli.BoolFlag{
						Name: "node-validation",
					},
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdBlxrBatchTX,
			},
			{
				Name:  "getinfo",
				Usage: "query information on running instance",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdGetInfo,
			},
			{
				Name:  "listpeers",
				Usage: "list current connected peers",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdListPeers,
			},
			{
				Name:  "txservice",
				Usage: "query information related to the TxStore",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdTxService,
			},
			{
				Name: "stop",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "auth-header",
						Required: true,
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdStop,
			},
			{
				Name:  "version",
				Usage: "query information related to the TxService",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdVersion,
			},
			{
				Name:  "status",
				Usage: "query gateway status",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header"},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdStatus,
			},
			{
				Name:  "listsubscriptions",
				Usage: "query information related to the Subscriptions",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdListSubscriptions,
			},
			{
				Name:  "disconnectinboundpeer",
				Usage: "disconnect inbound node from gateway",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "ip",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "port",
						Required: false,
					},
					&cli.StringFlag{
						Name:     "enode",
						Required: false,
					},
					&cli.StringFlag{
						Name: "auth-header",
					},
				},
				Before: checkEmptyProvidedHeader,
				Action: cmdDisconnectInboundPeer,
			},
		},
		Flags: []cli.Flag{
			utils.GRPCHostFlag,
			utils.GRPCPortFlag,
			utils.GRPCUserFlag,
			utils.GRPCPasswordFlag,
			utils.GRPCAuthFlag,
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func checkEmptyProvidedHeader(ctx *cli.Context) error {
	authHeader := ctx.String("auth-header")
	if ctx.IsSet("auth-header") && authHeader == "" {
		return fmt.Errorf("auth-header provided but is empty")
	}
	return nil
}

func cmdStop(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Stop(callCtx, &pb.StopRequest{AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not run stop: %v", err)
	}
	return nil
}

func cmdVersion(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Version(callCtx, &pb.VersionRequest{AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch version: %v", err)
	}
	return nil
}

func cmdNewTXs(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewStreamFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.NewTxs(callCtx, &pb.TxsRequest{Filters: ctx.String("filters"), Includes: ctx.StringSlice("include"), AuthHeader: ctx.String("auth-header")})
			if err != nil {
				return nil, err
			}
			for {
				tx, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("newTxs error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("newTxs error in recv: ", err)
					break
				}
				fmt.Println(tx)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to newTxs: %v", err)
	}

	return nil
}

func cmdPendingTXs(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.PendingTxs(callCtx, &pb.TxsRequest{Filters: ctx.String("filters"), Includes: ctx.StringSlice("include"), AuthHeader: ctx.String("auth-header")})
			if err != nil {
				return nil, err
			}
			for {
				tx, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("pendingTxs error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("pendingTxs error in recv: ", err)
					break
				}
				fmt.Println(tx)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to pendingTxs: %v", err)
	}

	return nil
}

func cmdNewBlocks(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.NewBlocks(callCtx, &pb.BlocksRequest{Includes: ctx.StringSlice("include"), AuthHeader: ctx.String("auth-header")})
			if err != nil {
				return nil, err
			}
			for {
				block, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("newBlocks error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("newBlocks error in recv: ", err)
					break
				}
				fmt.Println(block)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to newBlocks: %v", err)
	}

	return nil
}

func cmdBdnBlocks(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			stream, err := client.BdnBlocks(callCtx, &pb.BlocksRequest{Includes: ctx.StringSlice("include"), AuthHeader: ctx.String("auth-header")})
			if err != nil {
				return nil, err
			}
			for {
				block, err := stream.Recv()
				if err == io.EOF {
					fmt.Println("bdnBlocks error EOF: ", err)
					break
				}
				if err != nil {
					fmt.Println("bdnBlocks error in recv: ", err)
					break
				}
				fmt.Println(block)
			}
			return nil, nil
		},
	)
	if err != nil {
		return fmt.Errorf("err subscribing to bdnBlocks: %v", err)
	}

	return nil
}

func cmdBlxrTX(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrTx(callCtx, &pb.BlxrTxRequest{Transaction: ctx.String("transaction"), AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not process blxr tx: %v", err)
	}
	return nil
}

func cmdDisconnectInboundPeer(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.DisconnectInboundPeer(callCtx, &pb.DisconnectInboundPeerRequest{PeerIp: ctx.String("ip"), PeerPort: ctx.Int64("port"), PublicKey: ctx.String("enode"), AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not process disconnect inbound node: %v", err)
	}
	return nil
}

func cmdBlxrBatchTX(ctx *cli.Context) error {
	transactions := ctx.StringSlice("transactions")
	var txsAndSenders []*pb.TxAndSender
	for _, transaction := range transactions {
		var ethTx ethtypes.Transaction
		txBytes, err := types.DecodeHex(transaction)
		if err != nil {
			fmt.Printf("Error - failed to decode transaction %v: %v. continue..", transaction, err)
			continue
		}
		err = ethTx.UnmarshalBinary(txBytes)
		if err != nil {
			e := rlp.DecodeBytes(txBytes, &ethTx)
			if e != nil {
				fmt.Printf("Error - failed to decode transaction bytes %v: %v. continue..", transaction, err)
				continue
			}
		}

		ethSender, err := ethtypes.Sender(ethtypes.NewLondonSigner(ethTx.ChainId()), &ethTx)
		if err != nil {
			fmt.Printf("Error - failed to get sender from the transaction %v: %v. continue..", transaction, err)
		}
		txsAndSenders = append(txsAndSenders, &pb.TxAndSender{Transaction: transaction, Sender: ethSender.Bytes()})

	}
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.BlxrBatchTX(callCtx, &pb.BlxrBatchTXRequest{
				TransactionsAndSenders: txsAndSenders,
				NonceMonitoring:        ctx.Bool("nonce-monitoring"),
				NextValidator:          ctx.Bool("next-validator"),
				ValidatorsOnly:         ctx.Bool("validators-only"),
				Fallback:               int32(ctx.Int("fallback")),
				NodeValidation:         ctx.Bool("node-validation"),
				SendingTime:            time.Now().UnixNano(),
				AuthHeader:             ctx.String("auth-header"),
			})
		},
	)
	if err != nil {
		return fmt.Errorf("err sending transaction: %v", err)
	}

	return nil
}

func cmdGetInfo(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdTxService(*cli.Context) error {
	fmt.Printf("left to do:")
	return nil
}

func cmdListSubscriptions(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Subscriptions(callCtx, &pb.SubscriptionsRequest{AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch peers: %v", err)
	}
	return nil
}

func cmdListPeers(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Peers(callCtx, &pb.PeersRequest{AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not fetch peers: %v", err)
	}
	return nil
}

func cmdStatus(ctx *cli.Context) error {
	err := rpc.GatewayConsoleCall(
		config.NewGRPCFromCLI(ctx),
		func(callCtx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Status(callCtx, &pb.StatusRequest{AuthHeader: ctx.String("auth-header")})
		},
	)
	if err != nil {
		return fmt.Errorf("could not get status: %v", err)
	}
	return nil
}
