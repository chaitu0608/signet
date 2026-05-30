// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import {Script, console2} from "forge-std/Script.sol";

interface ISchemaRegistry {
    function register(
        string calldata schema,
        address resolver,
        bool revocable
    ) external returns (bytes32);
}

/// @notice One-time registration of Forge dev-rep schema on Base Sepolia EAS.
contract RegisterSchema is Script {
    address constant SCHEMA_REGISTRY = 0x4200000000000000000000000000000000000020;

    string constant FORGE_SCHEMA =
        "bytes32 commitHash,bytes32 leafHash,string repo,string branch,uint16 qualityScore,string aiSummary,string[] securityFlags,string category";

    function run() external {
        uint256 pk = vm.envUint("DEPLOYER_PRIVATE_KEY");
        vm.startBroadcast(pk);

        bytes32 uid = ISchemaRegistry(SCHEMA_REGISTRY).register(FORGE_SCHEMA, address(0), true);

        vm.stopBroadcast();

        console2.log("EAS_SCHEMA_UID=", vm.toString(uid));
        console2.log("Paste into .env: EAS_SCHEMA_UID=", vm.toString(uid));
    }
}
