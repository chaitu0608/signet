// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {ForgePulseAnchor} from "../src/ForgePulseAnchor.sol";
import {ContribSBT} from "../src/ContribSBT.sol";

/// @notice Deploys Signet's onchain backbone:
///   - ForgePulseAnchor: Merkle proof-of-existence batcher
///   - ContribSBT: optional soulbound contributor token
/// EAS attestations use the canonical contract at 0x4200..0021;
/// register the schema once via RegisterSchema.s.sol.
contract Deploy is Script {
    function run() external {
        uint256 pk = vm.envUint("DEPLOYER_PRIVATE_KEY");
        address deployer = vm.addr(pk);

        vm.startBroadcast(pk);
        ForgePulseAnchor anchor = new ForgePulseAnchor(deployer);
        ContribSBT sbt = new ContribSBT(deployer);
        vm.stopBroadcast();

        console2.log("ANCHOR_ADDR=", address(anchor));
        console2.log("SBT_ADDR=", address(sbt));
        console2.log("RELAYER=", deployer);
        console2.log("Next: forge script script/RegisterSchema.s.sol --rpc-url $RPC_URL --broadcast");
    }
}
