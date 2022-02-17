package syscontracts

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

func ForkForSystemContracts(config *params.ChainConfig, blockNumber *big.Int, statedb *state.StateDB) {
	if config == nil || blockNumber == nil || statedb == nil {
		return
	}

	var network string
	switch GenesisHash {
	case params.ParibuNetMainnetGenesisHash:
		network = mainnet
	case params.ParibuNetTestnetGenesisHash:
		network = testnet
	}

	if config.IsOnX(blockNumber) {
		doForkForSystemContract(xFork[network], blockNumber, statedb, network)
	}
}

func doForkForSystemContract(upgrade *Fork, blockNumber *big.Int, statedb *state.StateDB, network string) {

	logger := log.New("fork-for-system-contract", network)

	if upgrade == nil {
		logger.Info("Empty fork config", "height", blockNumber.String())
		return
	}

	logger.Info(fmt.Sprintf("Apply fork %s at height %d", upgrade.Name, blockNumber.Int64()))
	for _, config := range upgrade.ForkConfigs {
		logger.Info(fmt.Sprintf("Fork system contract %s to commit %s", config.SysContractAddr.String(), config.ForkCommit))

		if config.BeforeFork != nil {
			err := config.BeforeFork(blockNumber, config.SysContractAddr, statedb)
			if err != nil {
				panic(fmt.Errorf("system contract address: %s, execute beforeFork error: %s", config.SysContractAddr.String(), err.Error()))
			}
		}

		newContractCode, err := hex.DecodeString(config.ContractCode)
		if err != nil {
			panic(fmt.Errorf("failed to decode new system contract code: %s", err.Error()))
		}
		statedb.SetCode(config.SysContractAddr, newContractCode)

		if config.AfterFork != nil {
			err := config.AfterFork(blockNumber, config.SysContractAddr, statedb)
			if err != nil {
				panic(fmt.Errorf("system contract address: %s, execute afterFork error: %s", config.SysContractAddr.String(), err.Error()))
			}
		}
	}
}
