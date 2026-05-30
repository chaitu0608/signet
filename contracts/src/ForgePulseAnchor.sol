// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title ForgePulseAnchor — Merkle roots of git push events
contract ForgePulseAnchor {
    address public relayer;
    uint256 public batchCount;

    mapping(uint256 => bytes32) public roots;
    mapping(uint256 => uint64) public timestamps;
    mapping(uint256 => uint32) public leafCounts;

    event Anchored(uint256 indexed batchId, bytes32 root, uint64 ts, uint32 leaves);
    event RelayerUpdated(address indexed relayer);

    constructor(address _relayer) {
        relayer = _relayer;
    }

    function setRelayer(address _relayer) external {
        require(msg.sender == relayer, "not relayer");
        relayer = _relayer;
        emit RelayerUpdated(_relayer);
    }

    function anchor(bytes32 root, uint32 leaves) external {
        require(msg.sender == relayer, "not relayer");
        uint256 id = batchCount++;
        roots[id] = root;
        timestamps[id] = uint64(block.timestamp);
        leafCounts[id] = leaves;
        emit Anchored(id, root, uint64(block.timestamp), leaves);
    }

    function verify(
        uint256 batchId,
        bytes32 leaf,
        bytes32[] calldata proof
    ) external view returns (bool) {
        bytes32 computed = leaf;
        for (uint256 i = 0; i < proof.length; i++) {
            bytes32 sibling = proof[i];
            if (computed > sibling) {
                (computed, sibling) = (sibling, computed);
            }
            computed = keccak256(abi.encodePacked(computed, sibling));
        }
        return computed == roots[batchId];
    }
}
