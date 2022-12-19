package bouleuterion

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/bouleuterion/syscontracts"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
)

const (
	checkpointInterval = 1024 // Number of blocks after which to save the snapshot to the database
	inMemorySnapshots  = 128  // Number of recent snapshots to keep in memory
	inMemorySignatures = 4096 // Number of recent block signatures to keep in memory

	defaultEpochLength = uint64(120) // Default number of blocks of checkpoint to update validatorSet from contract, 10 * 60 / 5(period) = 120 blocks per 10 minutes

	extraVanity = 32 // Fixed number of extra-data prefix bytes reserved for signer vanity
	extraSeal   = 65 // Fixed number of extra-data suffix bytes reserved for signer seal
)

var (
	uncleHash  = types.CalcUncleHash(nil) // Always Keccak256(RLP([])) as uncles are meaningless outside of PoW.
	diffInTurn = big.NewInt(2)            // Block difficulty for in-turn signatures
	diffNoTurn = big.NewInt(1)            // Block difficulty for out-of-turn signatures
)

// SignerFn is a signer callback function to request a header to be signed by a
// backing account.
type SignerFn func(accounts.Account, string, []byte) ([]byte, error)

// ecrecover extracts the Ethereum account address from a signed header.
func ecrecover(header *types.Header, sigCache *lru.ARCCache, chainId *big.Int) (common.Address, error) {
	// If the signature's already cached, return that
	hash := header.Hash()
	if address, known := sigCache.Get(hash); known {
		return address.(common.Address), nil
	}
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		return common.Address{}, errMissingSignature
	}
	signature := header.Extra[len(header.Extra)-extraSeal:]

	// Recover the public key and the Ethereum address
	pubkey, err := crypto.Ecrecover(SealHash(header, chainId).Bytes(), signature)
	if err != nil {
		return common.Address{}, err
	}
	var signer common.Address
	copy(signer[:], crypto.Keccak256(pubkey[1:])[12:])

	sigCache.Add(hash, signer)
	return signer, nil
}

// Bouleuterion is the consensus engine of Bouleuterion
type Bouleuterion struct {
	chainConfig *params.ChainConfig        // Chain config
	config      *params.BouleuterionConfig // Consensus engine configuration parameters for bouleuterion consensus
	genesisHash common.Hash
	db          ethdb.Database // Database to store and retrieve snapshot checkpoints

	recentSnaps *lru.ARCCache // Snapshots for recent block to speed up
	signatures  *lru.ARCCache // Signatures of recent blocks to speed up mining
	signer      types.Signer

	val                common.Address // Ethereum address of the signing key
	signFn             SignerFn       // Signer function to authorize hashes with
	signTxFn           SignerTxFn
	secureRandomNumber SecureRandomNumber
	lock               sync.RWMutex // Protects the signer fields

	ethAPI *ethapi.PublicBlockChainAPI

	abiMap map[string]abi.ABI // Interactive with system contracts

	// The fields below are for testing only
	fakeDiff bool // Skip difficulty verifications
}

// New creates a Bouleuterion consensus engine.
func New(
	chainConfig *params.ChainConfig,
	db ethdb.Database,
	ethAPI *ethapi.PublicBlockChainAPI,
	genesisHash common.Hash,
) *Bouleuterion {
	// get bouleuterion config
	bouleuterionConfig := chainConfig.Bouleuterion

	// Set any missing consensus parameters to their defaults
	if bouleuterionConfig != nil && bouleuterionConfig.Epoch == 0 {
		bouleuterionConfig.Epoch = defaultEpochLength
	}

	// Allocate the snapshot caches and create the engine
	recentSnaps, err := lru.NewARC(inMemorySnapshots)
	if err != nil {
		panic(err)
	}
	signatures, err := lru.NewARC(inMemorySignatures)
	if err != nil {
		panic(err)
	}
	c := &Bouleuterion{
		chainConfig: chainConfig,
		config:      bouleuterionConfig,
		genesisHash: genesisHash,
		db:          db,
		ethAPI:      ethAPI,
		recentSnaps: recentSnaps,
		signatures:  signatures,
		abiMap:      syscontracts.AbiMap,
		signer:      types.NewEIP155Signer(chainConfig.ChainID),
	}

	return c
}

// Author implements consensus.Engine, returning the SystemAddress
func (b *Bouleuterion) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules.
func (b *Bouleuterion) VerifyHeader(chain consensus.ChainHeaderReader, header *types.Header, seal bool) error {
	return b.verifyHeader(chain, header, nil)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (b *Bouleuterion) VerifyHeaders(chain consensus.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := b.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (b *Bouleuterion) verifyHeader(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	if header.Number == nil {
		return errUnknownBlock
	}
	number := header.Number.Uint64()

	// Don't waste time checking blocks from the future
	if header.Time > uint64(time.Now().Unix()) {
		return consensus.ErrFutureBlock
	}
	// Check that the extra-data contains the vanity, validators and signature.
	if len(header.Extra) < extraVanity {
		return errMissingVanity
	}
	if len(header.Extra) < extraVanity+extraSeal {
		return errMissingSignature
	}
	// check if current block is an epoch block
	isEpoch := number%b.config.Epoch == 0

	// Ensure that the extra-data contains a signer list on checkpoint, but none otherwise
	signersBytes := len(header.Extra) - extraVanity - extraSeal
	if !isEpoch && signersBytes != 0 {
		return errExtraValidators
	}

	if isEpoch && signersBytes%validatorBytesLength != 0 {
		return errInvalidSpanValidators
	}

	// Ensure that the mix digest is zero as we don't have fork protection currently
	if header.MixDigest != (common.Hash{}) {
		return errInvalidMixDigest
	}
	// Ensure that the block doesn't contain any uncles which are meaningless in PoA
	if header.UncleHash != uncleHash {
		return errInvalidUncleHash
	}
	// Ensure that the block's difficulty is meaningful (may not be correct at this point)
	if number > 0 {
		if header.Difficulty == nil || (header.Difficulty.Cmp(diffInTurn) != 0 && header.Difficulty.Cmp(diffNoTurn) != 0) {
			return errInvalidDifficulty
		}
	}
	// If all checks passed, validate any special fields for hard forks
	if err := misc.VerifyForkHashes(chain.Config(), header, false); err != nil {
		return err
	}
	// All basic checks passed, verify cascading fields
	return b.verifyCascadingFields(chain, header, parents)
}

// verifyCascadingFields verifies all the header fields that are not standalone,
// rather depend on a batch of previous headers. The caller may optionally pass
// in a batch of parents (ascending order) to avoid looking those up from the
// database. This is useful for concurrently verifying a batch of new headers.
func (b *Bouleuterion) verifyCascadingFields(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// The genesis block is the always valid dead-end
	number := header.Number.Uint64()
	if number == 0 {
		return nil
	}

	var parent *types.Header
	if len(parents) > 0 {
		parent = parents[len(parents)-1]
	} else {
		parent = chain.GetHeader(header.ParentHash, number-1)
	}

	if parent == nil || parent.Number.Uint64() != number-1 || parent.Hash() != header.ParentHash {
		return consensus.ErrUnknownAncestor
	}

	// Verify that the gas limit is <= 2^63-1
	capacity := uint64(0x7fffffffffffffff)
	if header.GasLimit > capacity {
		return fmt.Errorf("invalid gasLimit: have %v, max %v", header.GasLimit, capacity)
	}
	// Verify that the gasUsed is <= gasLimit
	if header.GasUsed > header.GasLimit {
		return fmt.Errorf("invalid gasUsed: have %d, gasLimit %d", header.GasUsed, header.GasLimit)
	}

	// Verify that the gas limit remains within allowed bounds
	diff := int64(parent.GasLimit) - int64(header.GasLimit)
	if diff < 0 {
		diff *= -1
	}
	limit := parent.GasLimit / params.GasLimitBoundDivisor

	if uint64(diff) >= limit || header.GasLimit < params.MinGasLimit {
		return fmt.Errorf("invalid gas limit: have %d, want %d += %d", header.GasLimit, parent.GasLimit, limit)
	}

	// All basic checks passed, verify the seal and return
	return b.verifySeal(chain, header, parents)
}

// snapshot retrieves the authorization snapshot at a given point in time.
func (b *Bouleuterion) snapshot(chain consensus.ChainHeaderReader, number uint64, hash common.Hash, parents []*types.Header) (*Snapshot, error) {
	// Search for a snapshot in memory or on disk for checkpoints
	var (
		headers []*types.Header
		snap    *Snapshot
	)

	for snap == nil {
		// If an in-memory snapshot was found, use that
		if s, ok := b.recentSnaps.Get(hash); ok {
			snap = s.(*Snapshot)
			break
		}

		// If an on-disk checkpoint snapshot can be found, use that
		if number%checkpointInterval == 0 {
			if s, err := loadSnapshot(b.config, b.signatures, b.db, hash, b.ethAPI); err == nil {
				log.Trace("Loaded snapshot from disk", "number", number, "hash", hash)
				snap = s
				break
			}
		}

		// If we're at the genesis, snapshot the initial state.
		if number == 0 {
			checkpoint := chain.GetHeaderByNumber(number)
			if checkpoint != nil {
				// get checkpoint data
				hash := checkpoint.Hash()

				validatorBytes := checkpoint.Extra[extraVanity : len(checkpoint.Extra)-extraSeal]
				// get validators from headers
				validators, err := ParseValidators(validatorBytes)
				if err != nil {
					return nil, err
				}
				// new snap shot
				snap = newSnapshot(b.config, b.signatures, number, hash, validators, b.ethAPI)
				if err := snap.store(b.db); err != nil {
					return nil, err
				}
				log.Info("Stored checkpoint snapshot to disk", "number", number, "hash", hash)
				break
			}
		}

		// No snapshot for this header, gather the header and move backward
		var header *types.Header
		if len(parents) > 0 {
			// If we have explicit parents, pick from there (enforced)
			header = parents[len(parents)-1]
			if header.Hash() != hash || header.Number.Uint64() != number {
				return nil, consensus.ErrUnknownAncestor
			}
			parents = parents[:len(parents)-1]
		} else {
			// No explicit parents (or no more left), reach out to the database
			header = chain.GetHeader(hash, number)
			if header == nil {
				return nil, consensus.ErrUnknownAncestor
			}
		}
		headers = append(headers, header)
		number, hash = number-1, header.ParentHash
	}
	// check if snapshot is nil
	if snap == nil {
		return nil, fmt.Errorf("unknown error while retrieving snapshot at block number %v", number)
	}

	// Previous snapshot found, apply any pending headers on top of it
	for i := 0; i < len(headers)/2; i++ {
		headers[i], headers[len(headers)-1-i] = headers[len(headers)-1-i], headers[i]
	}

	snap, err := snap.apply(headers, chain, parents, b.chainConfig.ChainID)
	if err != nil {
		return nil, err
	}
	b.recentSnaps.Add(snap.Hash, snap)

	// If we've generated a new checkpoint snapshot, save to disk
	if snap.Number%checkpointInterval == 0 && len(headers) > 0 {
		if err = snap.store(b.db); err != nil {
			return nil, err
		}
		log.Trace("Stored snapshot to disk", "number", snap.Number, "hash", snap.Hash)
	}
	return snap, err
}

// VerifyUncles implements consensus.Engine, always returning an error for any
// uncles as this consensus mechanism doesn't permit uncles.
func (b *Bouleuterion) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	if len(block.Uncles()) > 0 {
		return errors.New("uncles not allowed")
	}
	return nil
}

// verifySeal checks whether the signature contained in the header satisfies the
// consensus protocol requirements. The method accepts an optional list of parent
// headers that aren't yet part of the local blockchain to generate the snapshots
// from.
func (b *Bouleuterion) verifySeal(chain consensus.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	// Verifying the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	// Retrieve the snapshot needed to verify this header and cache it
	snap, err := b.snapshot(chain, number-1, header.ParentHash, parents)
	if err != nil {
		return err
	}

	// Resolve the authorization key and check against validators
	signer, err := ecrecover(header, b.signatures, b.chainConfig.ChainID)
	if err != nil {
		return err
	}

	if signer != header.Coinbase {
		return errCoinBaseMisMatch
	}

	if _, ok := snap.Validators[signer]; !ok {
		return errUnauthorizedValidator
	}

	for seen, recent := range snap.Recents {
		if recent == signer {
			// Signer is among recents, only fail if the current block doesn't shift it out
			if limit := uint64(len(snap.Validators)/2 + 1); seen > number-limit {
				return errRecentlySigned
			}
		}
	}

	// Ensure that the difficulty corresponds to the turn-ness of the signer
	if !b.fakeDiff {
		inturn := snap.inturn(signer)
		if inturn && header.Difficulty.Cmp(diffInTurn) != 0 {
			return errWrongDifficulty
		}
		if !inturn && header.Difficulty.Cmp(diffNoTurn) != 0 {
			return errWrongDifficulty
		}
	}

	return nil
}

// Prepare implements consensus.Engine, preparing all the consensus fields of the
// header for running the transactions on top.
func (b *Bouleuterion) Prepare(chain consensus.ChainHeaderReader, header *types.Header) error {
	header.Coinbase = b.val
	header.Nonce = types.BlockNonce{}

	number := header.Number.Uint64()
	snap, err := b.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}

	// Set the correct difficulty
	header.Difficulty = CalcDifficulty(snap, b.val)

	// Ensure the extra data has all it's components
	if len(header.Extra) < extraVanity-nextForkHashSize {
		header.Extra = append(header.Extra, bytes.Repeat([]byte{0x00}, extraVanity-nextForkHashSize-len(header.Extra))...)
	}
	header.Extra = header.Extra[:extraVanity-nextForkHashSize]
	nextForkHash := forkid.NextForkHash(b.chainConfig, b.genesisHash, number)
	header.Extra = append(header.Extra, nextForkHash[:]...)

	//Check if current block is the epoch block. If yes, get validators from the contract, sort them and add them in the header.Extra
	if number%b.config.Epoch == 0 {
		newValidators, err := b.getValidatorSet(header.ParentHash)
		if err != nil {
			return err
		}
		for _, validator := range newValidators {
			header.Extra = append(header.Extra, validator.Bytes()...)
		}
	}

	// add extra seal space
	header.Extra = append(header.Extra, make([]byte, extraSeal)...)

	// Mix digest is reserved for now, set to empty
	header.MixDigest = common.Hash{}

	// Ensure the timestamp has the correct delay
	parent := chain.GetHeader(header.ParentHash, number-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Time = parent.Time + b.config.Period
	if header.Time < uint64(time.Now().Unix()) {
		header.Time = uint64(time.Now().Unix())
	}
	return nil
}

// Finalize implements consensus.Engine, ensuring no uncles are set, nor block
// rewards given.
func (b *Bouleuterion) Finalize(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB, txs *[]*types.Transaction,
	uncles []*types.Header, receipts *[]*types.Receipt, systemTxs *[]*types.Transaction, usedGas *uint64) error {
	// warn if not in majority fork
	number := header.Number.Uint64()
	snap, err := b.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}
	nextForkHash := forkid.NextForkHash(b.chainConfig, b.genesisHash, number)
	if !snap.isMajorityFork(hex.EncodeToString(nextForkHash[:])) {
		log.Debug("there is a possible fork, and your client is not the majority. Please check...", "nextForkHash", hex.EncodeToString(nextForkHash[:]))
	}
	cx := chainContext{Chain: chain, bouleuterion: b}
	// If the block is a epoch end block, verify the validator list
	// The verification can only be done when the state is ready, it can't be done in VerifyHeader.
	if header.Number.Uint64()%b.config.Epoch == 0 {
		newValidators, err := b.getValidatorSet(header.ParentHash)
		if err != nil {
			return err
		}

		validatorsBytes := make([]byte, len(newValidators)*validatorBytesLength)
		for i, validator := range newValidators {
			copy(validatorsBytes[i*validatorBytesLength:], validator.Bytes())
		}

		extraSuffix := len(header.Extra) - extraSeal
		if !bytes.Equal(header.Extra[extraVanity:extraSuffix], validatorsBytes) {
			return errMismatchingEpochValidators
		}
	}

	if header.Number.Cmp(common.Big1) == 0 {
		err := b.initSystemContracts(state, header, cx, txs, receipts, systemTxs, usedGas, false)
		if err != nil {
			log.Error("init system contract failed")
		}
	}

	if header.Difficulty.Cmp(diffInTurn) != 0 {
		lazyVal, err := b.trySlashValidator(header, state, snap, cx, txs, receipts, systemTxs, usedGas, false)
		if err != nil {
			log.Error("slash validator failed", "block hash", header.Hash(), "address", lazyVal)
		}
	}

	val := header.Coinbase

	//check and verify system txs
	tx := make([]*types.Transaction, len(*systemTxs))
	copy(tx, *systemTxs)

	if *tx[0].To() == syscontracts.RandaoContractAddr {
		from, _ := types.Sender(b.signer, tx[0])
		msg := b.getSystemMessage(from, *tx[0].To(), tx[0].Data(), common.Big0)
		err = b.applyTransaction(msg, state, header, cx, txs, receipts, systemTxs, usedGas, false)
		if err != nil {
			return err
		}
	}

	err = b.tryDistributeToSystem(val, state, header, cx, txs, receipts, systemTxs, usedGas, false)
	if err != nil {
		return err
	}
	if header.Number.Uint64()%b.config.Epoch == 0 {
		err = b.epochCall(header, state, cx, txs, receipts, systemTxs, usedGas, false)
		if err != nil {
			return err
		}
	}

	if len(*systemTxs) > 0 {
		return errors.New("the length of systemTxs do not match")
	}
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)
	return nil
}

// FinalizeAndAssemble implements consensus.Engine, ensuring no uncles are set,
// nor block rewards given, and returns the final block.
func (b *Bouleuterion) FinalizeAndAssemble(chain consensus.ChainHeaderReader, header *types.Header, state *state.StateDB,
	txs []*types.Transaction, uncles []*types.Header, receipts []*types.Receipt) (*types.Block, []*types.Receipt, error) {

	cx := chainContext{Chain: chain, bouleuterion: b}
	//Check if there is no transaction in the current block.
	if txs == nil {
		txs = make([]*types.Transaction, 0)
	}
	//Check if there is no receipt in the current block.
	if receipts == nil {
		receipts = make([]*types.Receipt, 0)
	}
	if header.Number.Uint64()%b.config.Epoch == 0 {
		_, err := b.getValidatorSet(header.ParentHash)
		if err != nil {
			log.Error("get validator set failed", "error", err)
		}
	}
	//Check if current block is the genesis block, init contracts.
	if header.Number.Cmp(common.Big1) == 0 {
		err := b.initSystemContracts(state, header, cx, &txs, &receipts, nil, &header.GasUsed, true)
		if err != nil {
			log.Error("init system contract failed", "error", err)
		}
	}
	val := header.Coinbase

	// Check if current validator is not in turn
	if header.Difficulty.Cmp(diffInTurn) != 0 {
		number := header.Number.Uint64()
		snap, err := b.snapshot(chain, number-1, header.ParentHash, nil)
		if err != nil {
			log.Error("Can't get snap")
		}
		lazyVal, err := b.trySlashValidator(header, state, snap, cx, &txs, &receipts, nil, &header.GasUsed, true)
		if err != nil {
			log.Error("slash validator failed", "block hash", header.Hash(), "address", lazyVal, "error", err)
		}
	}

	err := b.tryGenSecureRandom(header, state, cx, &txs, &receipts, nil, &header.GasUsed, true, header.ParentHash, val)
	if err != nil {
		log.Error("generate random number failed", "error", err)
	}
	// Distribute gas fees in the current block.
	err = b.tryDistributeToSystem(b.val, state, header, cx, &txs, &receipts, nil, &header.GasUsed, true)
	if err != nil {
		return nil, nil, err
	}

	if header.Number.Uint64()%b.config.Epoch == 0 {
		err = b.epochCall(header, state, cx, &txs, &receipts, nil, &header.GasUsed, true)
		if err != nil {
			log.Error("epoch call failed", "error", err)
		}
	}

	if header.GasLimit < header.GasUsed {
		return nil, nil, errors.New("gas consumption of system txs exceed the gas limit")
	}
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	header.UncleHash = types.CalcUncleHash(nil)

	// Assemble and return the final block for sealing
	return types.NewBlock(header, txs, nil, receipts, trie.NewStackTrie(nil)), receipts, nil
}

// Authorize injects a private key into the consensus engine to mint new blocks
// with.
func (b *Bouleuterion) Authorize(val common.Address, signFn SignerFn, signTxFn SignerTxFn, secureRandomNumber SecureRandomNumber) {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.val = val
	b.signFn = signFn
	b.signTxFn = signTxFn
	b.secureRandomNumber = secureRandomNumber
}

// Seal implements consensus.Engine, attempting to create a sealed block using
// the local signing credentials.
func (b *Bouleuterion) Seal(chain consensus.ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error {
	header := block.Header()

	// Sealing the genesis block is not supported
	number := header.Number.Uint64()
	if number == 0 {
		return errUnknownBlock
	}
	// For 0-period chains, refuse to seal empty blocks (no reward but would spin sealing)
	if b.config.Period == 0 && len(block.Transactions()) == 0 {
		return errors.New("sealing paused while waiting for transactions")
	}
	// Don't hold the val fields for the entire sealing procedure
	b.lock.RLock()
	val, signFn := b.val, b.signFn
	b.lock.RUnlock()

	snap, err := b.snapshot(chain, number-1, header.ParentHash, nil)
	if err != nil {
		return err
	}
	// Bail out if we're unauthorized to sign a block
	if _, authorized := snap.Validators[val]; !authorized {
		return errUnauthorizedValidator
	}

	// If we're amongst the recent signers, wait for the next block
	for seen, recent := range snap.Recents {
		if recent == val {
			// Signer is among recents, only wait if the current block doesn't shift it out
			if limit := uint64(len(snap.Validators)/2 + 1); number < limit || seen > number-limit {
				log.Info("Signed recently, must wait for others")
				return nil
			}
		}
	}

	// Sweet, the protocol permits us to sign the block, wait for our time
	delay := b.delay(snap, header)

	// Sign all the things!
	sig, err := signFn(accounts.Account{Address: val}, accounts.MimetypeBouleuterion, BouleuterionRLP(header, b.chainConfig.ChainID))
	if err != nil {
		return err
	}
	copy(header.Extra[len(header.Extra)-extraSeal:], sig)

	// Wait until sealing is terminated or delay timeout.
	log.Trace("Waiting for slot to sign and propagate", "delay", common.PrettyDuration(delay))
	go func() {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}

		select {
		case results <- block.WithSeal(header):
		default:
			log.Warn("Sealing result is not read by miner", "sealhash", SealHash(header, b.chainConfig.ChainID))
		}
	}()

	return nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func (b *Bouleuterion) CalcDifficulty(chain consensus.ChainHeaderReader, time uint64, parent *types.Header) *big.Int {
	snap, err := b.snapshot(chain, parent.Number.Uint64(), parent.Hash(), nil)
	if err != nil {
		return nil
	}
	return CalcDifficulty(snap, b.val)
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficulty
// that a new block should have based on the previous blocks in the chain and the
// current signer.
func CalcDifficulty(snap *Snapshot, signer common.Address) *big.Int {
	if snap.inturn(signer) {
		return new(big.Int).Set(diffInTurn)
	}
	return new(big.Int).Set(diffNoTurn)
}

// SealHash returns the hash of a block prior to it being sealed.
func (b *Bouleuterion) SealHash(header *types.Header) common.Hash {
	return SealHash(header, b.chainConfig.ChainID)
}

// Close implements consensus.Engine. It's a noop for bouleuterion as there are no background threads.
func (b *Bouleuterion) Close() error {
	return nil
}

// APIs implements consensus.Engine, returning the user facing RPC API to query snapshot.
func (b *Bouleuterion) APIs(chain consensus.ChainHeaderReader) []rpc.API {
	return []rpc.API{{
		Namespace: "bouleuterion",
		Version:   "1.0",
		Service:   &API{chain: chain, bouleuterion: b},
		Public:    false,
	}}
}

// SealHash returns the hash of a block prior to it being sealed.
func SealHash(header *types.Header, chainId *big.Int) (hash common.Hash) {
	hasher := sha3.NewLegacyKeccak256()
	encodeSigHeader(hasher, header, chainId)
	hasher.(crypto.KeccakState).Read(hash[:])
	return hash
}

// BouleuterionRLP returns the rlp bytes which needs to be signed for the bouleuterion
// sealing. The RLP to sign consists of the entire header apart from the 65 byte signature
// contained at the end of the extra data.
//
// Note, the method requires the extra data to be at least 65 bytes, otherwise it
// panics. This is done to avoid accidentally using both forms (signature present
// or not), which could be abused to produce different hashes for the same header.
func BouleuterionRLP(header *types.Header, chainId *big.Int) []byte {
	b := new(bytes.Buffer)
	encodeSigHeader(b, header, chainId)
	return b.Bytes()
}

func encodeSigHeader(w io.Writer, header *types.Header, chainId *big.Int) {
	err := rlp.Encode(w, []interface{}{
		chainId,
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-65], // this will panic if extra is too short, should check before calling encodeSigHeader
		header.MixDigest,
		header.Nonce,
	})
	if err != nil {
		panic("can't encode: " + err.Error())
	}
}
