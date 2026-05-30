// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title ContribSBT — non-transferable contributor credentials
contract ContribSBT {
    string public name = "ForgePulse Contributor";
    string public symbol = "FPC";
    address public relayer;

    uint256 private _nextId = 1;
    mapping(uint256 => address) public ownerOf;
    mapping(uint256 => bytes32) public commitHash;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => bool)) private _operator;

    event Transfer(address indexed from, address indexed to, uint256 indexed tokenId);
    event RelayerUpdated(address indexed relayer);

    constructor(address _relayer) {
        relayer = _relayer;
    }

    function mintBatch(address[] calldata devs, bytes32[] calldata hashes) external {
        require(msg.sender == relayer, "not relayer");
        require(devs.length == hashes.length, "length");
        for (uint256 i = 0; i < devs.length; i++) {
            uint256 id = _nextId++;
            ownerOf[id] = devs[i];
            commitHash[id] = hashes[i];
            balanceOf[devs[i]]++;
            emit Transfer(address(0), devs[i], id);
        }
    }

    function tokenURI(uint256 tokenId) external view returns (string memory) {
        require(ownerOf[tokenId] != address(0), "no token");
        return string(abi.encodePacked("forgepulse:commit:", _toHex(commitHash[tokenId])));
    }

    function _toHex(bytes32 data) internal pure returns (string memory) {
        bytes memory alphabet = "0123456789abcdef";
        bytes memory str = new bytes(66);
        str[0] = "0";
        str[1] = "x";
        for (uint256 i = 0; i < 32; i++) {
            str[2 + i * 2] = alphabet[uint8(data[i] >> 4)];
            str[3 + i * 2] = alphabet[uint8(data[i] & 0x0f)];
        }
        return string(str);
    }

    function transferFrom(address, address, uint256) external pure {
        revert("soulbound");
    }

    function safeTransferFrom(address, address, uint256, bytes calldata) external pure {
        revert("soulbound");
    }

    function approve(address, uint256) external pure {
        revert("soulbound");
    }

    function setApprovalForAll(address, bool) external pure {
        revert("soulbound");
    }
}
