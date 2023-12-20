[bsc readme](README.original.md)

# BSC Builder

This project implements the BEP-322: Builder API Specification for BNB Smart Chain.

This project represents a minimal implementation of the protocol and is provided as is. We make no guarantees regarding its functionality or security.

See also: https://github.com/bnb-chain/BEPs/pull/322

# Usage

Builder-related settings are configured in the `config.toml` file. The following is an example of a `config.toml` file:

```
[Eth.Miner.Bidder]
Enable = true
Account = {{BUILDER_ADDRESS}}
DelayLeftOver = {{DELAY_LEFT_OVER}}

[[Eth.Miner.Bidder.Validators]]
Address = {{VALIDATOR_ADDRESS}}
URL = {{VALIDATOR_URL}}
...
```

- `Enable`: Whether to enable the builder.
- `Account`: The account address to unlock of the builder.
- `DelayLeftOver`: Submit bid no later than `DelayLeftOver` before the next block time.
- `Validators`: A list of validators to bid for.
  - `Address`: The address of the validator.
  - `URL`: The URL of the validator.

## License

The bsc library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the `COPYING.LESSER` file.

The bsc binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also
included in our repository in the `COPYING` file.
