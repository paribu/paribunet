package bouleuterion

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
)

const (
	nextForkHashSize = 4

	validatorBytesLength = common.AddressLength
)

const (
	wiggleTime  = 500 * time.Millisecond
	backOffTime = wiggleTime / 2
)

type SignerTxFn func(accounts.Account, *types.Transaction, *big.Int) (*types.Transaction, error)
type SecureRandomNumber func(account accounts.Account, identifier []byte) ([]byte, error)

func (b *Bouleuterion) genNewSeedId() []byte {
	newCipher := make([]byte, 32)
	rand.Read(newCipher)
	return newCipher
}

func (b *Bouleuterion) getCommit(newSecureSeed []byte) []byte {
	return crypto.Keccak256(newSecureSeed)
}

func (b *Bouleuterion) IsLocalBlock(header *types.Header) bool {
	return b.val == header.Coinbase
}

func (b *Bouleuterion) SealRecently(chain consensus.ChainReader, parent *types.Header) (bool, error) {
	snap, err := b.snapshot(chain, parent.Number.Uint64(), parent.ParentHash, nil)
	if err != nil {
		return true, err
	}

	if _, authorized := snap.Validators[b.val]; !authorized {
		return true, errUnauthorizedValidator
	}

	number := parent.Number.Uint64() + 1
	for seen, recent := range snap.Recents {
		if recent == b.val {
			if limit := uint64(len(snap.Validators)/2 + 1); number < limit || seen > number-limit {
				return true, nil
			}
		}
	}
	return false, nil
}

func isToSystemContract(to common.Address) bool {
	return systemContracts[to]
}

func (b *Bouleuterion) IsSystemTransaction(tx *types.Transaction, header *types.Header) (bool, error) {
	if tx.To() == nil {
		return false, nil
	}
	sender, err := types.Sender(b.signer, tx)
	if err != nil {
		return false, errors.New("UnAuthorized transaction")
	}
	if sender == header.Coinbase && isToSystemContract(*tx.To()) && tx.GasPrice().Cmp(big.NewInt(0)) == 0 {
		return true, nil
	}
	return false, nil
}

func (b *Bouleuterion) IsSystemContract(to *common.Address) bool {
	if to == nil {
		return false
	}
	return isToSystemContract(*to)
}

func (b *Bouleuterion) VerifySeal(chain consensus.ChainReader, header *types.Header) error {
	return b.verifySeal(chain, header, nil)
}

func (b *Bouleuterion) Delay(chain consensus.ChainReader, header *types.Header) *time.Duration {
	number := header.Number.Uint64()
	snap, err := b.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return nil
	}
	delay := b.delay(snap, header)
	return &delay
}

func (b *Bouleuterion) EnoughDistance(chain consensus.ChainReader, header *types.Header) bool {
	snap, err := b.snapshot(chain, header.Number.Uint64()-1, header.ParentHash, nil)
	if err != nil {
		return true
	}
	return snap.enoughDistance(b.val, header)
}

func (b *Bouleuterion) delay(snap *Snapshot, header *types.Header) time.Duration {
	delay := time.Until(time.Unix(int64(header.Time), 0))
	if header.Difficulty.Cmp(diffNoTurn) == 0 {
		wiggle := time.Duration(len(snap.Validators)/2+1) * wiggleTime
		delay += backOffTime + time.Duration(rand.Int63n(int64(wiggle)))
	}
	return delay
}

func (b *Bouleuterion) getSystemMessage(from, toAddress common.Address, data []byte, value *big.Int) callmsg {
	return callmsg{
		ethereum.CallMsg{
			From:     from,
			Gas:      params.MaxSysTxsGas,
			GasPrice: big.NewInt(0),
			Value:    value,
			To:       &toAddress,
			Data:     data,
		},
	}
}

func (b *Bouleuterion) applyTransaction(
	msg callmsg,
	state *state.StateDB,
	header *types.Header,
	chainContext core.ChainContext,
	txs *[]*types.Transaction, receipts *[]*types.Receipt,
	receivedTxs *[]*types.Transaction, usedGas *uint64, mining bool,
) (err error) {
	nonce := state.GetNonce(msg.From())
	expectedTx := types.NewTransaction(nonce, *msg.To(), msg.Value(), msg.Gas(), msg.GasPrice(), msg.Data())
	expectedHash := b.signer.Hash(expectedTx)
	if msg.From() == b.val && mining {
		if expectedTx, err = b.signTxFn(accounts.Account{Address: msg.From()}, expectedTx, b.chainConfig.ChainID); err != nil {
			return err
		}
	} else {
		if receivedTxs == nil || len(*receivedTxs) == 0 || (*receivedTxs)[0] == nil {
			return errors.New("expected to receive an actual transaction, but got nothing")
		}
		actualTx := (*receivedTxs)[0]
		if !bytes.Equal(b.signer.Hash(actualTx).Bytes(), expectedHash.Bytes()) {
			return fmt.Errorf("actual transaction hash %v, expected transaction hash %v", actualTx.Hash().String(), expectedHash.String())
		}
		expectedTx = actualTx
		*receivedTxs = (*receivedTxs)[1:]
	}
	var gasUsed uint64
	state.Prepare(expectedTx.Hash(), common.Hash{}, len(*txs))
	if gasUsed, err = applyMessage(msg, state, header, b.chainConfig, chainContext); err != nil {
		return err
	}
	*txs = append(*txs, expectedTx)
	var rootHash []byte
	if b.chainConfig.IsByzantium(header.Number) {
		state.Finalise(true)
	} else {
		rootHash = state.IntermediateRoot(b.chainConfig.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gasUsed
	receipt := types.NewReceipt(rootHash, false, *usedGas)
	receipt.TxHash = expectedTx.Hash()
	receipt.GasUsed = gasUsed

	receipt.Logs = state.GetLogs(expectedTx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = state.BlockHash()
	receipt.BlockNumber = header.Number
	receipt.TransactionIndex = uint(state.TxIndex())
	*receipts = append(*receipts, receipt)
	state.SetNonce(msg.From(), nonce+1)
	return nil
}

func applyMessage(
	msg callmsg,
	state *state.StateDB,
	header *types.Header,
	chainConfig *params.ChainConfig,
	chainContext core.ChainContext,
) (uint64, error) {
	context := core.NewEVMBlockContext(header, chainContext, nil)
	vmenv := vm.NewEVM(context, vm.TxContext{Origin: msg.From(), GasPrice: big.NewInt(0)}, state, chainConfig, vm.Config{})
	ret, returnGas, err := vmenv.Call(
		vm.AccountRef(msg.From()),
		*msg.To(),
		msg.Data(),
		msg.Gas(),
		msg.Value(),
	)
	if err != nil {
		log.Error("apply message failed", "msg", string(ret), "message", common.Bytes2Hex(ret), "err", err)
	}
	return msg.Gas() - returnGas, err
}

type chainContext struct {
	Chain        consensus.ChainHeaderReader
	bouleuterion consensus.Engine
}

func (c chainContext) Engine() consensus.Engine {
	return c.bouleuterion
}

func (c chainContext) GetHeader(hash common.Hash, number uint64) *types.Header {
	return c.Chain.GetHeader(hash, number)
}

type callmsg struct {
	ethereum.CallMsg
}

func (m callmsg) From() common.Address { return m.CallMsg.From }
func (m callmsg) Nonce() uint64        { return 0 }
func (m callmsg) CheckNonce() bool     { return false }
func (m callmsg) To() *common.Address  { return m.CallMsg.To }
func (m callmsg) GasPrice() *big.Int   { return m.CallMsg.GasPrice }
func (m callmsg) Gas() uint64          { return m.CallMsg.Gas }
func (m callmsg) Value() *big.Int      { return m.CallMsg.Value }
func (m callmsg) Data() []byte         { return m.CallMsg.Data }

func isZero(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] != 0 {
			return false
		}
	}
	return true
}
