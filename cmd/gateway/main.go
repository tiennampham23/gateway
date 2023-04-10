package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path"
	"syscall"

	upscale_client "github.com/bloXroute-Labs/upscale-client"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/nodes"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "gateway",
		Usage: "run a GO gateway",
		Flags: []cli.Flag{
			utils.ExternalIPFlag,
			utils.PortFlag,
			utils.SDNURLFlag,
			utils.CACertURLFlag,
			utils.RegistrationCertDirFlag,
			utils.WSFlag,
			utils.WSPortFlag,
			utils.HTTPPortFlag,
			utils.EnvFlag,
			utils.LogLevelFlag,
			utils.LogFileLevelFlag,
			utils.LogMaxSizeFlag,
			utils.LogMaxAgeFlag,
			utils.LogMaxBackupsFlag,
			utils.TxTraceEnabledFlag,
			utils.TxTraceMaxFileSizeFlag,
			utils.TxTraceMaxBackupFilesFlag,
			utils.AvoidPrioritySendingFlag,
			utils.RelayHostsFlag,
			utils.DataDirFlag,
			utils.GRPCFlag,
			utils.GRPCHostFlag,
			utils.GRPCPortFlag,
			utils.GRPCUserFlag,
			utils.GRPCPasswordFlag,
			utils.BlockchainNetworkFlag,
			utils.EnodesFlag,
			utils.EthWSUriFlag,
			utils.MultiNode,
			utils.BeaconENRFlag,
			utils.BeaconMultiaddrFlag,
			utils.PrysmGRPCFlag,
			utils.BlocksOnlyFlag,
			utils.GensisFilePath,
			utils.AllTransactionsFlag,
			utils.PrivateKeyFlag,
			utils.NodeTypeFlag,
			utils.GatewayModeFlag,
			utils.DisableProfilingFlag,
			utils.FluentDFlag,
			utils.FluentdHostFlag,
			utils.ManageWSServer,
			utils.LogNetworkContentFlag,
			utils.WSTLSFlag,
			utils.MEVBuilderURIFlag,
			utils.MEVMinerURIFlag,
			utils.MEVMaxProfitBuilder,
			utils.MEVBundleMethodNameFlag,
			utils.SendBlockConfirmation,
			utils.MegaBundleProcessing,
			utils.TerminalTotalDifficulty,
			utils.EnableDynamicPeers,
			utils.ForwardTransactionEndpoint,
			utils.ForwardTransactionMethod,
			utils.PolygonMainnetHeimdallEndpoint,
			utils.BSCTransactionHoldDuration,
			utils.BSCTransactionPassedDueDuration,
		},
		Action: runGateway,
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func runGateway(c *cli.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if !c.Bool(utils.DisableProfilingFlag.Name) {
		go func() {
			log.Infof("pprof http server is running on 0.0.0.0:6060 - %v", "http://localhost:6060/debug/pprof")
			log.Error(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	bxConfig, err := config.NewBxFromCLI(c)
	if err != nil {
		return err
	}

	err = log.Init(bxConfig.Config, version.BuildVersion)
	if err != nil {
		return err
	}

	dataDir := c.String(utils.DataDirFlag.Name)
	ethConfig, gatewayPublicKey, err := network.NewPresetEthConfigFromCLI(c, dataDir)
	if err != nil {
		return err
	}

	var blockchainPeers []types.NodeEndpoint
	var prysmEndpoint types.NodeEndpoint
	var prysmAddr string
	blockchainNetwork := c.String(utils.BlockchainNetworkFlag.Name)

	for _, blockchainPeerInfo := range ethConfig.StaticPeers {
		var endpoint types.NodeEndpoint
		if blockchainPeerInfo.Enode != nil {
			endpoint = utils.EnodeToNodeEndpoint(blockchainPeerInfo.Enode, blockchainNetwork)
		} else if blockchainPeerInfo.Multiaddr != nil {
			endpoint = utils.MultiaddrToNodeEndoint(*blockchainPeerInfo.Multiaddr, blockchainNetwork)
			prysmEndpoint = endpoint
		}

		blockchainPeers = append(blockchainPeers, endpoint)

		if blockchainPeerInfo.PrysmAddr != "" {
			prysmAddr = blockchainPeerInfo.PrysmAddr
		}
	}

	sslCerts, sdn, err := nodes.InitSDN(bxConfig, blockchainPeers, nodes.GeneratePeers(ethConfig.StaticPeers), len(ethConfig.StaticEnodes()))
	if err != nil {
		return err
	}

	startupBeaconNode := bxConfig.GatewayMode.IsBDN() && len(ethConfig.BeaconNodes()) > 0
	startupBlockchainClient := startupBeaconNode || len(ethConfig.StaticEnodes()) > 0 || bxConfig.EnableDynamicPeers // if beacon node running we need to receive txs also
	startupPrysmClient := bxConfig.GatewayMode.IsBDN() && prysmAddr != ""

	var bridge blockchain.Bridge
	if startupBlockchainClient || startupBeaconNode || startupPrysmClient {
		bridge = blockchain.NewBxBridge(eth.Converter{}, startupBeaconNode)
	} else {
		bridge = blockchain.NewNoOpBridge(eth.Converter{})
	}

	if bxConfig.ManageWSServer && !bxConfig.WebsocketEnabled && !bxConfig.WebsocketTLSEnabled {
		return fmt.Errorf("websocket server must be enabled using --ws or --ws-tls if --manage-ws-server is enabled")
	}
	wsManager := eth.NewEthWSManager(ethConfig.StaticPeers, eth.NewWSProvider, bxgateway.WSProviderTimeout)
	if (bxConfig.WebsocketEnabled || bxConfig.WebsocketTLSEnabled) && !ethConfig.ValidWSAddr() {
		log.Warn("websocket server enabled but no valid websockets endpoint specified via --eth-ws-uri nor --multi-node: only newTxs and bdnBlocks feeds are available")
	}
	if bxConfig.ManageWSServer && !ethConfig.ValidWSAddr() {
		return fmt.Errorf("if websocket server management is enabled, a valid websocket address must be provided")
	}

	gateway, err := nodes.NewGateway(ctx, bxConfig, bridge, wsManager, blockchainPeers, ethConfig.StaticPeers, gatewayPublicKey, sdn, sslCerts, len(ethConfig.StaticEnodes()), c.String(utils.PolygonMainnetHeimdallEndpoint.Name), c.Int(utils.BSCTransactionHoldDuration.Name), c.Int(utils.BSCTransactionPassedDueDuration.Name))
	if err != nil {
		return err
	}

	if err = gateway.Run(); err != nil {
		// TODO close the gateway while notify all other go routine (bridge, ws server, ...)
		log.Errorf("closing gateway with err %v", err)
		log.Exit(0)
	}

	// Required for beacon node and prysm to sync
	ethChain := eth.NewChain(ctx, ethConfig.IgnoreBlockTimeout)

	var blockchainServer *eth.Server
	if startupBlockchainClient {
		log.Infof("starting blockchain client with config for network ID: %v", ethConfig.Network)

		// TODO: use resolver to get public IP if externalIP flag is omitted
		port, externalIP := c.Int(utils.PortFlag.Name), net.ParseIP(c.String(utils.ExternalIPFlag.Name))

		dynamicPeers := int(sdn.AccountModel().InboundNodeConnections.MsgQuota.Limit)
		if !bxConfig.EnableDynamicPeers {
			dynamicPeers = 0
		}

		blockchainServer, err = eth.NewServerWithEthLogger(ctx, port, externalIP, ethConfig, ethChain, bridge, dataDir, wsManager, dynamicPeers)
		if err != nil {
			return nil
		}

		if err = blockchainServer.AddEthLoggerFileHandler(bxConfig.Config.FileName); err != nil {
			log.Warnf("skipping reconfiguration of eth p2p server logger due to error: %v", err)
		}

		if err = blockchainServer.Start(); err != nil {
			return nil
		}
		if dynamicPeers > 0 {
			// we initialize upscale with the gw node id, because upscale accept only string with 64 chars we pad the node id with 0s in the start
			// example: node id 12345e07-1234-1234-1234-464d308375df will be 000000000000000000000000000012345e07-1234-1234-1234-464d308375df
			log.Infof("starting upscale to manage %v dynamic peers", dynamicPeers)
			upscale_client.Init(fmt.Sprintf("%064s", string(sdn.NodeID())))
		}
	} else {
		log.Infof("skipping starting blockchain client as no enodes have been provided")
	}

	var beaconNode *beacon.Node
	if startupBeaconNode {
		var genesisPath string
		localGenesisFile := path.Join(dataDir, "genesis.ssz")
		if c.IsSet(utils.GensisFilePath.Name) {
			localGenesisFile = c.String(utils.GensisFilePath.Name)
			genesisPath = localGenesisFile
		} else {
			genesisPath, err = downloadGenesisFile(c.String(utils.BlockchainNetworkFlag.Name), localGenesisFile)
			if err != nil {
				return err
			}
		}
		log.Info("connecting to beacon node using ", genesisPath)

		beaconNode, err := beacon.NewNode(ctx, c.String(utils.BlockchainNetworkFlag.Name), ethConfig, localGenesisFile, bridge)
		if err != nil {
			return err
		}

		if err = beaconNode.Start(); err != nil {
			return err
		}
	}

	var prysmClient *beacon.PrysmClient
	if startupPrysmClient {
		prysmClient = beacon.NewPrysmClient(ctx, ethConfig, prysmAddr, bridge, prysmEndpoint)
		prysmClient.Start()
	}

	<-sigc

	if blockchainServer != nil {
		blockchainServer.Stop()
	}

	if beaconNode != nil {
		beaconNode.Stop()
	}

	return nil
}

func downloadGenesisFile(network, genesisFilePath string) (string, error) {
	var genesisFileURL string
	switch network {
	case bxgateway.Mainnet:
		genesisFileURL = "https://github.com/eth-clients/eth2-mainnet/raw/master/genesis.ssz"
	case bxgateway.Ropsten:
		genesisFileURL = "https://github.com/eth-clients/merge-testnets/raw/main/ropsten-beacon-chain/genesis.ssz"
	case bxgateway.Zhejiang:
		genesisFileURL = "https://github.com/ethpandaops/withdrawals-testnet/raw/master/withdrawal-mainnet-shadowfork-3/custom_config_data/genesis.ssz"
	case bxgateway.Goerli:
		genesisFileURL = "https://github.com/eth-clients/goerli/raw/main/prater/genesis.ssz"

	default:
		return "", fmt.Errorf("beacon node is supported on ethereum mainnet and ropsten")
	}

	out, err := os.Create(genesisFilePath)
	if err != nil {
		return "", fmt.Errorf("failed creating %v file %v", genesisFilePath, err)
	}
	defer out.Close()

	resp, err := http.Get(genesisFileURL)
	if err != nil {
		return "", fmt.Errorf("failed calling server for genesis.ssz from %v %v", genesisFileURL, err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed downloading genesis.ssz from %v %v", genesisFileURL, err)
	}

	return genesisFileURL, nil
}
