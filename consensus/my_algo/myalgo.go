// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package myalgo

import (
  "github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"sync"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/consensus"
	"math/big"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/log"
  
	"encoding/binary"
	"math/rand"
	"fmt"
	"strconv"
	"io/ioutil"
	"os"
	"encoding/json"
  
	"github.com/Knetic/govaluate"
	"github.com/pkg/errors"  
)

// Ethash proof-of-work protocol constants.
var (
	FrontierBlockReward       = big.NewInt(6e+18) // Block reward in wei for successfully mining a block
	ByzantiumBlockReward      = big.NewInt(4e+18) // Block reward in wei for successfully mining a block upward from Byzantium
	ConstantinopleBlockReward = big.NewInt(2e+18) // Block reward in wei for successfully mining a block upward from Constantinople
	minerBlockReward               *big.Int = new(big.Int).Mul(big.NewInt(10), big.NewInt(1e+18))
	masternodeBlockReward          *big.Int = big.NewInt(2e+18) 
	developmentBlockReward         *big.Int = big.NewInt(1e+18)
  maxUncles                 = 2                 // Maximum number of uncles allowed in a single block
	allowedFutureBlockTime    = 15 * time.Second  // Max time from current time allowed for blocks, before they're considered future blocks

// error types into the consensus package.

type Problem struct {
	Index    int    `json:"index"`
	Equation string `json:"equation"`
}

func getProblems() []Problem {

	raw, err := ioutil.ReadFile("./problems.json")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var c []Problem
	json.Unmarshal(raw, &c)
	return c
}

func (p Problem) toString() string {
	return toJson(p)
}

func toJson(p interface{}) string {
	bytes, err := json.Marshal(p)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	return string(bytes)
}

var problems []Problem

// New creates a Clique proof-of-authority consensus engine with the initial
// signers set to the ones provided by the user.
func New(config *params.MyAlgoConfig, db ethdb.Database) *MyAlgo {
	// Set any missing consensus parameters to their defaults
	conf := *config
	problems = getProblems()
	return &MyAlgo{
		config:     &conf,
		db:         db,
	}
}

// Clique is the proof-of-authority consensus engine proposed to support the
// Ethereum testnet following the Ropsten attacks.
type MyAlgo struct {
	config *params.MyAlgoConfig // Consensus engine configuration parameters
	db     ethdb.Database       // Database to store and retrieve snapshot checkpoints
	lock   sync.RWMutex   // Protects the signer fields
}



// Author implements consensus.Engine, returning the header's coinbase as the
// proof-of-work verified author of the block.
func (MyAlgo *MyAlgo) Author(header *types.Header) (common.Address, error) {
	return header.Coinbase, nil
}

// VerifyHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (MyAlgo *MyAlgo) VerifyHeader(chain consensus.ChainReader, header *types.Header, seal bool) error {
	log.Info("will verfiyHeader")
	p, _ := getProblemFromHeader(header)
	result := solveProblem(p);
	correct := checkResult(result, header)
	if (correct){
		return nil
	}else {
		return errors.New("Invalid solution to the problem ")
	}
}

func checkResult(result float64, header *types.Header) bool {
	fmt.Print("result : ")
	fmt.Println(result)

	fmt.Print("to compare with  : ")
	fmt.Println(header.Nonce.Uint64())
	toCompare := header.Nonce.Uint64()
	return toCompare == uint64(result);

}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers
// concurrently. The method returns a quit channel to abort the operations and
// a results channel to retrieve the async verifications (the order is that of
// the input slice).
func (MyAlgo *MyAlgo) VerifyHeaders(chain consensus.ChainReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error){
	log.Info("will verfiyHeaders")
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for _, header := range headers {
			err := MyAlgo.VerifyHeader(chain, header, false)

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// VerifyUncles verifies that the given block's uncles conform to the consensus
// rules of a given engine.
func (MyAlgo *MyAlgo) VerifyUncles(chain consensus.ChainReader, block *types.Block) error {
	log.Info("will verfiy uncles")
	return nil
}

// VerifySeal checks whether the crypto seal on a header is valid according to
// the consensus rules of the given engine.
func (MyAlgo *MyAlgo)  VerifySeal(chain consensus.ChainReader, header *types.Header) error{
	log.Info("will verfiy VerifySeal")
	return nil
}

// Prepare initializes the consensus fields of a block header according to the
// rules of a particular engine. The changes are executed inline.
func (MyAlgo *MyAlgo) Prepare(chain consensus.ChainReader, header *types.Header) error{
	log.Info("will prepare the block")
	parent := chain.GetHeader(header.ParentHash, header.Number.Uint64()-1)
	if parent == nil {
		return consensus.ErrUnknownAncestor
	}
	header.Difficulty = MyAlgo.CalcDifficulty(chain, header.Time.Uint64(), parent)
	return nil
}

// CalcDifficulty is the difficulty adjustment algorithm. It returns the difficult
// that a new block should have.
func (MyAlgo *MyAlgo) CalcDifficulty(chain consensus.ChainReader, time uint64, parent *types.Header) *big.Int {
	//return calcDifficultyFrontier(time, parent)
	return calcDifficultyHomestead(time, parent)
}

// Some weird constants to avoid constant memory allocs for them.
var (
	expDiffPeriod = big.NewInt(100000)
	big1          = big.NewInt(1)
	big2          = big.NewInt(2)
	big9          = big.NewInt(9)
	big10         = big.NewInt(10)
	bigMinus99    = big.NewInt(-99)
)


// calcDifficultyHomestead is the difficulty adjustment algorithm. It returns
// the difficulty that a new block should have when created at time given the
// parent block's time and difficulty. The calculation uses the Homestead rules.
func calcDifficultyHomestead(time uint64, parent *types.Header) *big.Int {

	bigTime := new(big.Int).SetUint64(time)
	bigParentTime := new(big.Int).Set(parent.Time)

	// holds intermediate values to make the algo easier to read & audit
	x := new(big.Int)
	y := new(big.Int)

	// 1 - (block_timestamp - parent_timestamp) // 10
	x.Sub(bigTime, bigParentTime)
	x.Div(x, big10)
	x.Sub(big1, x)

	// max(1 - (block_timestamp - parent_timestamp) // 10, -99)
	if x.Cmp(bigMinus99) < 0 {
		x.Set(bigMinus99)
	}
	// (parent_diff + parent_diff // 2048 * max(1 - (block_timestamp - parent_timestamp) // 10, -99))
	y.Div(parent.Difficulty, params.DifficultyBoundDivisor)
	x.Mul(y, x)
	x.Add(parent.Difficulty, x)

	// minimum difficulty can ever be (before exponential factor)
	if x.Cmp(params.MinimumDifficulty) < 0 {
		x.Set(params.MinimumDifficulty)
	}
	// for the exponential factor
	periodCount := new(big.Int).Add(parent.Number, big1)
	periodCount.Div(periodCount, expDiffPeriod)

	// the exponential factor, commonly referred to as "the bomb"
	// diff = diff + 2^(periodCount - 2)
	if periodCount.Cmp(big1) > 0 {
		y.Sub(periodCount, big2)
		y.Exp(big2, y, nil)
		x.Add(x, y)
	}
	return x
}

// Finalize runs any post-transaction state modifications (e.g. block rewards)
// and assembles the final block.
// Note: The block header and state database might be updated to reflect any
// consensus rules that happen at finalization (e.g. block rewards).
func (MyAlgo *MyAlgo) Finalize(chain consensus.ChainReader, header *types.Header, state *state.StateDB, txs []*types.Transaction,
	uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error){
	log.Info("will Finalize the block")
  accumulateRewards(chain.Config(), state, header, uncles)
	header.Root = state.IntermediateRoot(chain.Config().IsEIP158(header.Number))
	b := types.NewBlock(header, txs, uncles, receipts)

	return b, nil
}

func getProblemFromHeader (header *types.Header) (Problem, int64){
	runes := []rune(header.ParentHash.String())
	index_in_hash := string(runes[0:3])
	index_in_decimal, _ := strconv.ParseInt(index_in_hash , 0, 64)
	index_in_decimal = index_in_decimal % 10
	return problems[index_in_decimal], index_in_decimal
}

func solveProblem(p Problem) (float64){
	expression, _ := govaluate.NewEvaluableExpression(p.Equation)
	result, _ := expression.Evaluate(nil)
	result_in_float := result.(float64)
	return result_in_float
}

// Seal generates a new block for the given input block with the local miner's
// seal place on top.
func (MyAlgo *MyAlgo) Seal(chain consensus.ChainReader, block *types.Block, stop <-chan struct{}) (*types.Block, error){
	log.Info("will Seal the block")
	//time.Sleep(15 * time.Second)
	header := block.Header()
	/*
	runes := []rune(header.ParentHash.String())
	index_in_hash := string(runes[0:3])
	index_in_decimal, _ := strconv.ParseInt(index_in_hash , 0, 64)
	index_in_decimal = index_in_decimal % 10
	*/
	p, index_in_decimal := getProblemFromHeader(header)

	fmt.Print("hash is : ")
	fmt.Print(header.ParentHash.String())
	fmt.Print("problem number is : ")
	fmt.Println(index_in_decimal)



	fmt.Print("problem is : ")
	fmt.Println(p.Equation)
	result_in_float := solveProblem(p)
	fmt.Print("solution is : ")
	fmt.Println(result_in_float)

	header.Nonce, header.MixDigest = getRequiredHeader(result_in_float)
	return block.WithSeal(header), nil
}

func getRequiredHeader(result float64) (types.BlockNonce, common.Hash){
	return getNonce(result), common.Hash{}
}

func getNonce(result float64) (types.BlockNonce) {
	var i uint64 = uint64(result)
	var n types.BlockNonce

	binary.BigEndian.PutUint64(n[:], i)
	return n
}



func rangeIn(low, hi int) int {

	return low + rand.Intn(hi-low)
}

// APIs returns the RPC APIs this consensus engine provides.
func (myAlgo *MyAlgo) APIs(chain consensus.ChainReader) []rpc.API {
	return []rpc.API{{
		Namespace: "myalgo",
		Version:   "1.0",
		Service:   &API{chain: chain, myAlgo: myAlgo},
		Public:    false,
	}}
}

// AccumulateRewards credits the coinbase of the given block with the mining
// reward. The total reward consists of the static block reward and rewards for
// included uncles. The coinbase of each uncle block is also rewarded.
func accumulateRewards(config *params.ChainConfig, state *state.StateDB, header *types.Header, uncles []*types.Header) {
	var blockReward = minerBlockReward // Set miner reward base
	var masternodeReward = masternodeBlockReward // Set masternode reward
	var developmentReward = developmentBlockReward // Set development reward
	
	if (header.Number.Int64() >= 1000000) && (header.Number.Int64() < 2000000) {
	        blockReward = big.NewInt(8e+18)
                masternodeReward = big.NewInt(2e+18)
                developmentReward = big.NewInt(1e+18)
	} else if (header.Number.Int64() >= 2000000) && (header.Number.Int64() < 3000000) {
	        blockReward = big.NewInt(640e+16)
                masternodeReward = big.NewInt(2e+18)
                developmentReward = big.NewInt(1e+18) 
	} else if (header.Number.Int64() >= 3000000) && (header.Number.Int64() < 4000000) {
	        blockReward = big.NewInt(510e+16)
                masternodeReward = big.NewInt(2e+18)
                developmentReward = big.NewInt(1e+18) 
	} else if (header.Number.Int64() >= 4000000) && (header.Number.Int64() < 5000000) {
	        blockReward = big.NewInt(400e+16)
                masternodeReward = big.NewInt(2e+18)
                developmentReward = big.NewInt(1e+18) 
	} else if (header.Number.Int64() >= 5000000) && (header.Number.Int64() < 6000000) {
	        blockReward = big.NewInt(320e+16)
                masternodeReward = big.NewInt(2e+18)
                developmentReward = big.NewInt(1e+18)
	} else if (header.Number.Int64() >= 6000000) && (header.Number.Int64()) < 7000000 {
	        blockReward = big.NewInt(250e+16)
                masternodeReward = big.NewInt(160e+16)
                developmentReward = big.NewInt(80e+16)
	} else if (header.Number.Int64() >= 7000000) && (header.Number.Int64() < 8000000) {
	        blockReward = big.NewInt(200e+16)
                masternodeReward = big.NewInt(130e+16)
                developmentReward = big.NewInt(65e+16)
	} else if (header.Number.Int64() >= 8000000) && (header.Number.Int64() < 9000000) {
	        blockReward = big.NewInt(160e+16)
                masternodeReward = big.NewInt(104e+16)
                developmentReward = big.NewInt(52e+16)
	} else if (header.Number.Int64() >= 9000000) && (header.Number.Int64() < 10000000) {
	        blockReward = big.NewInt(130e+16)
                masternodeReward = big.NewInt(83e+16)
                developmentReward = big.NewInt(415e+15)
	} else if (header.Number.Int64() >= 10000000) && (header.Number.Int64() < 11000000) {
	        blockReward = big.NewInt(100e+16)
                masternodeReward = big.NewInt(66e+16)
                developmentReward = big.NewInt(330e+15)
	} else if (header.Number.Int64() >= 11000000) && (header.Number.Int64() < 12000000) {
	        blockReward = big.NewInt(80e+16)
                masternodeReward = big.NewInt(53e+16)
                developmentReward = big.NewInt(265e+15)
	} else if (header.Number.Int64() >= 12000000) && (header.Number.Int64() < 13000000) {
	        blockReward = big.NewInt(65e+16)
                masternodeReward = big.NewInt(42e+16)
                developmentReward = big.NewInt(210e+15)
	} else if (header.Number.Int64() >= 13000000) && (header.Number.Int64() < 14000000) {
	        blockReward = big.NewInt(52e+16)
                masternodeReward = big.NewInt(34e+16)
                developmentReward = big.NewInt(170e+15)
	} else if (header.Number.Int64() >= 14000000) && (header.Number.Int64() < 15000000) {
	        blockReward = big.NewInt(42e+16)
                masternodeReward = big.NewInt(27e+16)
                developmentReward = big.NewInt(135e+15)
	} else if (header.Number.Int64() >= 15000000) && (header.Number.Int64() < 16000000) {
	        blockReward = big.NewInt(34e+16)
                masternodeReward = big.NewInt(22e+16)
                developmentReward = big.NewInt(110e+15)
	} else if (header.Number.Int64() >= 16000000) && (header.Number.Int64() < 17000000) {
	        blockReward = big.NewInt(27e+16)
                masternodeReward = big.NewInt(18e+16)
                developmentReward = big.NewInt(90e+15)
	} else if (header.Number.Int64() >= 17000000) && (header.Number.Int64() < 18000000) {
	        blockReward = big.NewInt(22e+16)
                masternodeReward = big.NewInt(14e+16)
                developmentReward = big.NewInt(70e+15)
	} else if (header.Number.Int64() >= 18000000) && (header.Number.Int64() < 19000000) {
	        blockReward = big.NewInt(18e+16)
                masternodeReward = big.NewInt(11e+16)
                developmentReward = big.NewInt(55e+15)
	} else if (header.Number.Int64() >= 19000000) && (header.Number.Int64() < 20000000) {
	        blockReward = big.NewInt(15e+16)
                masternodeReward = big.NewInt(9e+16)
                developmentReward = big.NewInt(45e+15)
	} else if (header.Number.Int64() >= 20000000) {
	        blockReward = big.NewInt(12e+16)
                masternodeReward = big.NewInt(7e+16)
                developmentReward = big.NewInt(35e+15)
	}
	if config.IsConstantinople(header.Number) {
		blockReward = ConstantinopleBlockReward
	}
	// Accumulate the rewards for the miner and any included uncles
	reward := new(big.Int).Set(blockReward)
	r := new(big.Int)
	for _, uncle := range uncles {
		r.Add(uncle.Number, big8)
		r.Sub(r, header.Number)
		r.Mul(r, blockReward)
		r.Div(r, big8)
		state.AddBalance(uncle.Coinbase, r)

		r.Div(blockReward, big32)
		reward.Add(reward, r)
	}
	state.AddBalance(header.Coinbase, reward)
	// Developement Fund Address
	state.AddBalance(common.HexToAddress("0xE2c8cbEc30c8513888F7A95171eA836f8802d981"), developmentReward)
	// Masternode Fund address
  state.AddBalance(common.HexToAddress("0xE19363Ffb51C62bEECd6783A2c9C5bfF5D4679ac"), masternodeReward)
}
