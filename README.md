## Paribu Net

Paribu net is a [go-ethereum](https://geth.ethereum.org/) based blockchain network. Paribu Net aims to create a new performance-oriented, eco-friendly, and business-focused blockchain architecture with the support of the underlying Proof-of-Authority (PoA) and Delegated Proof of Stake (DPoS) based consensus algorithms, internally referred as Bouleuterion consensus mechanism. Paribu Net is created to connect people and organizations providing new ways of communicating and cooperating with a smooth and reliable data transition. It is important to highlight that Paribu Net will follow the idea of decentralization by introducing a secure, reliable, and trustless transaction network. Paribu Net aims to achieve significantly low transaction fees for users. Paribu Net also supports EVM-based smart contracts to be able to develop decentralized applications such as DeFi, NFT, gaming metaverse, and domain naming systems.

If you want to get more details about Paribu Net you can read [whitepaper](https://net.paribu.com/download/whitepaper-en.pdf) or [lightpaper](https://net.paribu.com/download/lightpaper-en.pdf).

## Key Features

### Bouleuterion Consensus Mechanism

The implemented consensus mechanism of Paribu Net is called "Bouleuterion Consensus Mechanism" which is the combination of Delegated Proof of Stake (DPoS) and Proof of Authority (PoA). This mixture allows us to provide a decentralized selection process alongside the high performance (e.g., creating a new block in a few seconds (≈ 5 secs)).

The consensus consists of several different roles. The community will select 21 main validators, which are responsible for the production and validation of blocks, and 11 standby validators, which will be waiting in the queue to be one of the main validators. All the candidate nodes are ranked according to their staked PRB and the total stake amount on the candidates.

Every candidate validator must meet the minimum staking amount and other requirements. Any user in the network can invest in any main or standby validators. Therefore, stakers have a critical role in the selection of main and standby validators. Candidate validators can offer to share some part of their rewards with their contributors to promote themselves and raise their total staking amount. This competition process will encourage more candidate validators to be selected as a main validator successfully.

After the selection process, main validators start to produce or validate blocks. The transaction fees in the block are distributed as follows:

- %80 of the fees is equally shared among the main validators.

- The remaining %20 of the fees are equally shared among the standby validators.

Similar to Ethereum’s Clique consensus protocol, the main validators on the Bouleuterion consensus model take turns to produce blocks. In the future, these validators are planned to be chosen randomly by using the Randao system contract. This randomization protocol will allow us to eliminate potential attacks such as a greedy approach during the block generation phase. In addition to this, if any of the main validators act maliciously or fail to generate a block, the slashing system contract will take the necessary action.

Above all the features of the Bouleuterion consensus model, the main focus is to create decentralized decision-making and a community-driven network. For this purpose, a governance system contract has been implemented which allows the community to participate in the decision-making process for the ruleset in the blockchain.

### Components of Paribu Net

The concept of staking is a commonly used term in the blockchain ecosystem. In short, staking means that investors earn income by participating in the consensus process of their digital assets within the ecosystem. Staker is the name given to investors. Paribu Net consists of nodes, validators and stakers, as its core components.

**Nodes:** Refers to components that keep the distributed ledgers of Paribu Net.

**Validator:** Refers to components that create blocks of and validate transactions taking place on Paribu Net.

**Candidate Validator:** To verify transactions on Paribu Net, one must be a validator. Validators are selected from among previously nominated validators, taking into account their stakes.

**Main Validator:** Paribu Net processes transactions with 21 main validators. Each main validator creates a block and then broadcasts it in the blockchain.

**Standby Validator:** If one or more of the main validators can no longer perform their functions standby validators are able to replace them to perform the task of creating blocks. There is a maximum number of 11 standby validators in the system.

**Staker:** Refers to users who stake their PRB balances to either main validators or candidate validators and receive a portion of the block reward in exchange for being part of the validating process.

### System Contracts

System contracts have an important place in Paribu Net’s architecture. System contracts coming into effect with the launch of the mainnet are:

**Validator Contract:** This is a contract that is responsible for selecting validators and publishing the list of validators.

**Staking Contract:** This contract is responsible for PRB staking. It is also responsible for calculating and distributing the rewards to validators and stakers.

**Slashing Contract:** OThis contract monitors the block productions of validators and tracks any malicious behavior. The purpose of this contract is to punish any misbehavior that violates the rules of the consensus protocol by means of removing the Main Validator from the list of validators. Therefore, it enables security, availability of the validators, and honest network participation.

**Randao Contract:** With its distributed random number generating function, RANDAO was integrated with Paribu Net as a system contract. A secure random number is generated in each block where dApps will be able to use this service free of charge.

**Governance Contract:** It is through this contract that proposals for the improvement of the network are provided, and the decision-making process is managed via a participation in a voting process in proportion to the number of PRB coins owned.

**Reward Pool Contracts:** This contract holds 100 million PRB. In every block, 1 PRB is distributed to validator who validates curent block and to stakers who staked that validator.

System contracts can be reached via this [link](https://net.paribu.com/).

### Native Coin PRB

The Paribu Net’s ecosystem will run on the native coin PRB. PRB coins will be used for paying network fees and staking.

The maximum number of PRB coins available is limited to 600 million units. 500 million PRBs were sent to the respective addresses with the launch of the Paribu Net mainnet. While 100 million PRBs were reserved as validator rewards.

In each block, 1 PRB is paid to the Main Validator producing the block and it will take about 16 years for 100 million PRBs to be distributed to validators in exchange for the blocks generated. Also, all Main and Standby Validators get a share of the transaction fees.

## Building the source

Since Paribu Net is based on go-ethereum, the following parts are the same as go-ethereum.

For prerequisites and detailed build instructions please read the [Installation Instructions](https://geth.ethereum.org/docs/install-and-build/installing-geth).

Building `geth` requires both a Go (version 1.14 or later) and a C compiler. You can install
them using your favourite package manager. Once the dependencies are installed, run

```shell
make geth
```

or, to build the full suite of utilities:

```shell
make all
```

## Executables

The paribu-net project comes with several wrappers/executables found in the `cmd`
directory.

|  Command   | Description                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| :--------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **`geth`** | Our main Ethereum CLI client. It is the entry point into the Ethereum (main-, test- or private net), capable of running as a full node (default), archive node (retaining all historical state) or a light node (retrieving data live). It can be used by other processes as a gateway into the Ethereum via JSON RPC endpoints exposed on top of HTTP, WebSocket and/or IPC transports. `geth --help` and the [CLI page](https://geth.ethereum.org/docs/interface/command-line-options) for command line options. |
|   `clef`   | Stand-alone signing tool, which can be used as a backend signer for `geth`.                                                                                                                                                                                                                                                                                                                                                                                                                                        |
|  `devp2p`  | Utilities to interact with nodes on the networking layer, without running a full blockchain.                                                                                                                                                                                                                                                                                                                                                                                                                       |
|  `abigen`  | Source code generator to convert Ethereum contract definitions into easy to use, compile-time type-safe Go packages. It operates on plain [Ethereum contract ABIs](https://docs.soliditylang.org/en/develop/abi-spec.html) with expanded functionality if the contract bytecode is also available. However, it also accepts Solidity source files, making development much more streamlined. Please see our [Native DApps](https://geth.ethereum.org/docs/dapp/native-bindings) page for details.                  |
| `bootnode` | Stripped down version of our Ethereum client implementation that only takes part in the network node discovery protocol, but does not run any of the higher level application protocols. It can be used as a lightweight bootstrap node to aid in finding peers in private networks.                                                                                                                                                                                                                               |
|   `evm`    | Developer utility version of the EVM (Ethereum Virtual Machine) that is capable of running bytecode snippets within a configurable environment and execution mode. Its purpose is to allow isolated, fine-grained debugging of EVM opcodes (e.g. `evm --code 60ff60ff --debug run`).                                                                                                                                                                                                                               |
| `rlpdump`  | Developer utility tool to convert binary RLP ([Recursive Length Prefix](https://eth.wiki/en/fundamentals/rlp)) dumps (data encoding used by the Ethereum protocol both network as well as consensus wise) to user-friendlier hierarchical representation (e.g. `rlpdump --hex CE0183FFFFFFC4C304050583616263`).                                                                                                                                                                                                    |
| `puppeth`  | a CLI wizard that aids in creating a new Ethereum network.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |

## Running `geth`

Going through all the possible command line flags is out of scope here (please consult our
[CLI Wiki page](https://geth.ethereum.org/docs/interface/command-line-options)),
but we've enumerated a few common parameter combos to get you up to speed quickly
on how you can run your own `geth` instance.

### Configuration

As an alternative to passing the numerous flags to the `geth` binary, you can also pass a
configuration file via:

```shell
$ geth --config /path/to/your_config.toml
```

To get an idea how the file should look like you can use the `dumpconfig` subcommand to
export your existing configuration:

```shell
$ geth --your-favourite-flags dumpconfig
```

_Note: This works only with `geth` v1.6.0 and above._

### Programmatically interfacing `geth` nodes

As a developer, sooner rather than later you'll want to start interacting with `geth` and the
Paribu Net via your own programs and not manually through the console. To aid
this, `geth` has built-in support for a JSON-RPC based APIs ([standard APIs](https://eth.wiki/json-rpc/API)
and [`geth` specific APIs](https://geth.ethereum.org/docs/rpc/server)).
These can be exposed via HTTP, WebSockets and IPC (UNIX sockets on UNIX based
platforms, and named pipes on Windows).

The IPC interface is enabled by default and exposes all the APIs supported by `geth`,
whereas the HTTP and WS interfaces need to manually be enabled and only expose a
subset of APIs due to security reasons. These can be turned on/off and configured as
you'd expect.

HTTP based JSON-RPC API options:

- `--http` Enable the HTTP-RPC server
- `--http.addr` HTTP-RPC server listening interface (default: `localhost`)
- `--http.port` HTTP-RPC server listening port (default: `8545`)
- `--http.api` API's offered over the HTTP-RPC interface (default: `eth,net,web3`)
- `--http.corsdomain` Comma separated list of domains from which to accept cross origin requests (browser enforced)
- `--ws` Enable the WS-RPC server
- `--ws.addr` WS-RPC server listening interface (default: `localhost`)
- `--ws.port` WS-RPC server listening port (default: `8546`)
- `--ws.api` API's offered over the WS-RPC interface (default: `eth,net,web3`)
- `--ws.origins` Origins from which to accept websockets requests
- `--ipcdisable` Disable the IPC-RPC server
- `--ipcapi` API's offered over the IPC-RPC interface (default: `admin,debug,eth,miner,net,personal,shh,txpool,web3`)
- `--ipcpath` Filename for IPC socket/pipe within the datadir (explicit paths escape it)

You'll need to use your own programming environments' capabilities (libraries, tools, etc) to
connect via HTTP, WS or IPC to a `geth` node configured with the above flags and you'll
need to speak [JSON-RPC](https://www.jsonrpc.org/specification) on all transports. You
can reuse the same connection for multiple requests!

**Note: Please understand the security implications of opening up an HTTP/WS based
transport before doing so! Hackers on the internet are actively trying to subvert
Paribu Net nodes with exposed APIs! Further, all browser tabs can access locally
running web servers, so malicious web pages could try to subvert locally available
APIs!**

### Operating a private network

Some users may be want to set up a private network. To do this, users can follow this [link](https://net.paribu.com/) to set up private network.

## Contribution

Thank you for considering to help out with the source code! We welcome contributions
from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to paribu-net, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit
more complex changes though, please check up with the core devs first on [our Discord Server](https://discord.gg/invite/nthXNEv)
to ensure those changes are in line with the general philosophy of the project and/or get
some early feedback which can make both your efforts much lighter as well as our review
and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

- Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
  guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
- Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
  guidelines.
- Pull requests need to be based on and opened against the `master` branch.
- Commit messages should be prefixed with the package(s) they modify.
  - E.g. "eth, rpc: make trace configs optional"

Please see the [Developers' Guide](https://geth.ethereum.org/docs/developers/devguide)
for more details on configuring your environment, managing project dependencies, and
testing procedures.

## License

The paribu-net library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the `COPYING.LESSER` file.

The paribu-net binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also
included in our repository in the `COPYING` file.
