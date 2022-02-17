package bouleuterion

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/params"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/bouleuterion/syscontracts"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

var (
	systemContracts = map[common.Address]bool{
		syscontracts.ValidatorSetContractAddr: true,
		syscontracts.SlashingContractAddr:     true,
		syscontracts.RandaoContractAddr:       true,
		syscontracts.GovernanceContractAddr:   true,
	}
)

func (b *Bouleuterion) initSystemContracts(state *state.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool) error {

	method := "init"

	contracts := []common.Address{
		syscontracts.GovernanceContractAddr,
		syscontracts.ValidatorSetContractAddr,
	}

	governanceData, err := b.abiMap[syscontracts.GovernanceContractName].Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for init governance", "error", err)
		return err
	}

	validatorSetData, err := b.abiMap[syscontracts.ValidatorSetContractName].Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for init validatorSet", "error", err)
		return err
	}
	var msg callmsg
	for _, c := range contracts {
		if c == syscontracts.GovernanceContractAddr {
			msg = b.getSystemMessage(header.Coinbase, c, governanceData, common.Big0)
		} else if c == syscontracts.ValidatorSetContractAddr {
			msg = b.getSystemMessage(header.Coinbase, c, validatorSetData, common.Big0)
		}
		log.Info("initialize contract", "block hash", header.Hash(), "contract", c)
		err = b.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *Bouleuterion) getEpochInfo(blockHash common.Hash) ([]common.Address, error) {
	blockNr := rpc.BlockNumberOrHashWithHash(blockHash, false)
	method := "getEpochInfo"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data, err := b.abiMap[syscontracts.ValidatorSetContractName].Pack(method)
	if err != nil {
		log.Error("Unable to pack tx for getEpochInfo", "error", err)
		return nil, err
	}

	msgData := (hexutil.Bytes)(data)
	toAddress := syscontracts.ValidatorSetContractAddr
	gas := (hexutil.Uint64)(uint64(params.MaxSysTxsGas))
	result, err := b.ethAPI.Call(ctx, ethapi.CallArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, blockNr, nil)
	if err != nil {
		return nil, err
	}

	ret, err := b.abiMap[syscontracts.ValidatorSetContractName].Unpack(method, result)
	if err != nil {
		return nil, err
	}
	mainValidators, ok := ret[0].([]common.Address)
	if !ok {
		return nil, errors.New("can't get main validators")
	}
	sort.Sort(validatorsAscending(mainValidators))

	return mainValidators, nil
}

func (b *Bouleuterion) epochCall(header *types.Header, state *state.StateDB,
	chain core.ChainContext, txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool) error {

	method := "epochCall"
	data, err := b.abiMap[syscontracts.ValidatorSetContractName].Pack(method)
	if err != nil {
		log.Error("Can't pack data for epoch call", "error", err)
		return err
	}

	msg := b.getSystemMessage(header.Coinbase, syscontracts.ValidatorSetContractAddr, data, common.Big0)
	return b.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
}

func (b *Bouleuterion) tryDistributeToSystem(val common.Address, state *state.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool) error {

	fee := state.GetBalance(consensus.FeePool)
	if fee.Cmp(common.Big0) > 0 {
		state.AddBalance(header.Coinbase, fee)
		state.SetBalance(consensus.FeePool, common.Big0)
	}
	return b.distributeToSystem(fee, val, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
}

func (b *Bouleuterion) distributeToSystem(amount *big.Int, validator common.Address,
	state *state.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool) error {

	method := "deposit"
	data, err := b.abiMap[syscontracts.ValidatorSetContractName].Pack(method, validator)
	if err != nil {
		log.Error("Can't pack data for deposit", "err", err)
		return err
	}

	msg := b.getSystemMessage(header.Coinbase, syscontracts.ValidatorSetContractAddr, data, amount)
	return b.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
}

func (b *Bouleuterion) trySlashValidator(header *types.Header, state *state.StateDB, snap *Snapshot,
	cx core.ChainContext, txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction,
	usedGas *uint64, mining bool) (common.Address, error) {

	lazyVal := snap.suspectedValidator()
	signedRecently := false
	for _, recent := range snap.Recents {
		if recent == lazyVal {
			signedRecently = true
			break
		}
	}
	if !signedRecently {
		err := b.slashValidator(lazyVal, state, header, cx, txs, receipts, receivedTxs, usedGas, mining)
		if err != nil {
			log.Error("slash validator failed", "block hash", header.Hash(), "address", lazyVal)
		}
	}
	return lazyVal, nil
}

func (b *Bouleuterion) slashValidator(lazyVal common.Address, state *state.StateDB, header *types.Header, chain core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool) error {

	method := "blockMiss"

	data, err := b.abiMap[syscontracts.SlashingContractName].Pack(method, lazyVal)
	if err != nil {
		log.Error("Unable to pack tx for slash", "error", err)
		return err
	}

	msg := b.getSystemMessage(header.Coinbase, syscontracts.SlashingContractAddr, data, common.Big0)
	return b.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
}

func (b *Bouleuterion) tryGenSecureRandom(header *types.Header, state *state.StateDB,
	chain core.ChainContext, txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64,
	mining bool, blockHash common.Hash, validator common.Address) error {

	prevCommit, prevSeedId, err := b.getCommitAndSeedId(blockHash, validator)
	if err != nil {
		return err
	}
	revealSeed, _ := b.secureRandomNumber(accounts.Account{Address: b.val}, prevSeedId)
	if len(prevCommit) > 1 && !isZero(prevCommit) {
		if !bytes.Equal(b.getCommit(revealSeed), prevCommit) {
			return errors.New("cannot match revealSeed")
		}
	}
	newSeedId := b.genNewSeedId()
	newSecureSeed, _ := b.secureRandomNumber(accounts.Account{Address: b.val}, newSeedId)
	newCommit := b.getCommit(newSecureSeed)

	err = b.commitHashAndRevealPreviousHash(header, state, chain, txs, receipts, receivedTxs, usedGas, mining, newCommit, newSeedId, new(big.Int).SetBytes(revealSeed))
	if err != nil {
		return err
	}

	return nil
}

func (b *Bouleuterion) commitHashAndRevealPreviousHash(header *types.Header, state *state.StateDB,
	chain core.ChainContext, txs *[]*types.Transaction, receipts *[]*types.Receipt, receivedTxs *[]*types.Transaction, usedGas *uint64,
	mining bool, commitHash []byte, cipher []byte, revealPrevNumber *big.Int) error {

	method := "commitHashAndRevealPreviousHash"
	data, err := b.abiMap[syscontracts.RandaoContractName].Pack(method, toBytes32(commitHash), toBytes32(cipher), revealPrevNumber)
	if err != nil {
		log.Error("Can't pack data for commitHashAndRevealPreviousHash", "error", err)
		return err
	}

	msg := b.getSystemMessage(header.Coinbase, syscontracts.RandaoContractAddr, data, common.Big0)
	return b.applyTransaction(msg, state, header, chain, txs, receipts, receivedTxs, usedGas, mining)
}

func toBytes32(arr []byte) (result [32]byte) {
	copy(result[32-len(arr):], arr[:])
	return result
}

func (b *Bouleuterion) getCommitAndSeedId(blockHash common.Hash, validator common.Address) ([]byte, []byte, error) {

	blockNr := rpc.BlockNumberOrHashWithHash(blockHash, false)

	commit := [32]byte{}
	cipher := [32]byte{}

	method := "getCommitAndSeedId"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	data, err := b.abiMap[syscontracts.RandaoContractName].Pack(method, validator)
	if err != nil {
		log.Error("Unable to pack tx for getCommitAndSeedId", "error", err)
		return commit[:], cipher[:], err
	}

	msgData := (hexutil.Bytes)(data)
	toAddress := syscontracts.RandaoContractAddr
	gas := (hexutil.Uint64)(uint64(params.MaxSysTxsGas))
	result, err := b.ethAPI.Call(ctx, ethapi.CallArgs{
		Gas:  &gas,
		To:   &toAddress,
		Data: &msgData,
	}, blockNr, nil)
	if err != nil {
		return commit[:], cipher[:], err
	}

	ret, err := b.abiMap[syscontracts.RandaoContractName].Unpack(method, result)
	if err != nil {
		return commit[:], cipher[:], err
	}

	commit, ok := ret[0].([32]byte)
	if !ok {
		return commit[:], cipher[:], errors.New("cannot parse commit ret")
	}

	cipher, ok = ret[1].([32]byte)
	if !ok {
		return commit[:], cipher[:], errors.New("cannot parse cipher ret")
	}

	return commit[:], cipher[:], nil
}
