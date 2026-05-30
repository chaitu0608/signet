// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";
import {SignetReputation} from "../src/SignetReputation.sol";

contract DeployReputation is Script {
    function run() external {
        uint256 pk = vm.envUint("DEPLOYER_PRIVATE_KEY");
        address relayer = vm.envAddress("RELAYER_ADDRESS");

        vm.startBroadcast(pk);
        SignetReputation rep = new SignetReputation(relayer);
        vm.stopBroadcast();

        console2.log("SIGNET_REPUTATION_ADDR=", address(rep));
        console2.log("Paste into .env: REPUTATION_ADDR=", address(rep));
    }
}
