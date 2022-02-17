package syscontracts

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
)

var (
	ValidatorSetContractName = "validatorSet"
	SlashingContractName     = "slashing"
	RandaoContractName       = "randao"
	GovernanceContractName   = "governance"

	ValidatorSetContractAddr = common.HexToAddress("0x3400000000000000000000000000000000000000")
	SlashingContractAddr     = common.HexToAddress("0x3400000000000000000000000000000000000001")
	RandaoContractAddr       = common.HexToAddress("0x3400000000000000000000000000000000000002")
	StakingContractAddr      = common.HexToAddress("0x3400000000000000000000000000000000000003")
	GovernanceContractAddr   = common.HexToAddress("0x3400000000000000000000000000000000000004")
	RewardPoolContractAddr   = common.HexToAddress("0x3400000000000000000000000000000000000005")

	AbiMap map[string]abi.ABI

	GenesisHash common.Hash
)

const (
	mainnet = "Mainnet"
	testnet = "Testnet"
)

var (
	xFork = make(map[string]*Fork)
)

type Fork struct {
	Name        string
	ForkConfigs []*ForkConfig
}

type ForkConfig struct {
	SysContractAddr common.Address
	ContractCode    string
	ForkCommit      string
	BeforeFork      callHook
	AfterFork       callHook
}

type callHook func(blockNumber *big.Int, contractAddr common.Address, statedb *state.StateDB) error

func init() {
	AbiMap = make(map[string]abi.ABI)
	tmpABI, _ := abi.JSON(strings.NewReader(ValidatorSetABI))
	AbiMap[ValidatorSetContractName] = tmpABI
	tmpABI, _ = abi.JSON(strings.NewReader(SlashingABI))
	AbiMap[SlashingContractName] = tmpABI
	tmpABI, _ = abi.JSON(strings.NewReader(RandaoABI))
	AbiMap[RandaoContractName] = tmpABI
	tmpABI, _ = abi.JSON(strings.NewReader(GovernanceABI))
	AbiMap[GovernanceContractName] = tmpABI

	xFork[testnet] = &Fork{
		Name: "x",
		ForkConfigs: []*ForkConfig{
			{
				SysContractAddr: ValidatorSetContractAddr,
				ContractCode:    "",
			},
			{
				SysContractAddr: SlashingContractAddr,
				ContractCode:    "",
			},
			{
				SysContractAddr: RandaoContractAddr,
				ContractCode:    "",
			},
			{
				SysContractAddr: StakingContractAddr,
				ContractCode:    "",
			},
			{
				SysContractAddr: GovernanceContractAddr,
				ContractCode:    "",
			},
			{
				SysContractAddr: RewardPoolContractAddr,
				ContractCode:    "",
			},
		},
	}
}
