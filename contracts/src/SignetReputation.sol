// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

/// @title SignetReputation — ERC-8004-inspired on-chain reputation for proof-of-code
/// @notice Stores bounded feedback entries keyed by agent (developer wallet) address.
contract SignetReputation {
    struct Feedback {
        address client;
        int128 score;
        string tag;
        bytes32 evidenceRoot;
        uint64 timestamp;
        bytes32 eventLeaf;
    }

    address public relayer;
    mapping(address => Feedback[]) private _feedback;

    event FeedbackGiven(
        address indexed agent,
        address indexed client,
        int128 score,
        string tag,
        bytes32 evidenceRoot,
        bytes32 eventLeaf
    );
    event RelayerUpdated(address indexed relayer);

    error NotRelayer();
    error InvalidScore();

    modifier onlyRelayer() {
        if (msg.sender != relayer) revert NotRelayer();
        _;
    }

    constructor(address _relayer) {
        relayer = _relayer;
    }

    function setRelayer(address _relayer) external onlyRelayer {
        relayer = _relayer;
        emit RelayerUpdated(_relayer);
    }

    /// @notice Record reputation feedback for an agent after attestation.
    /// @param agent Developer or agent wallet receiving reputation.
    /// @param score Bounded quality score (0-100).
    /// @param tag Category tag (feature, fix, test, etc.).
    /// @param evidenceRoot Merkle root anchoring the attested push batch.
    /// @param eventLeaf Leaf hash for the specific attested event.
    function giveFeedback(
        address agent,
        int128 score,
        string calldata tag,
        bytes32 evidenceRoot,
        bytes32 eventLeaf
    ) external onlyRelayer {
        if (score < 0 || score > 100) revert InvalidScore();
        _feedback[agent].push(
            Feedback({
                client: msg.sender,
                score: score,
                tag: tag,
                evidenceRoot: evidenceRoot,
                timestamp: uint64(block.timestamp),
                eventLeaf: eventLeaf
            })
        );
        emit FeedbackGiven(agent, msg.sender, score, tag, evidenceRoot, eventLeaf);
    }

    function feedbackCount(address agent) external view returns (uint256) {
        return _feedback[agent].length;
    }

    function getFeedback(address agent, uint256 index)
        external
        view
        returns (
            address client,
            int128 score,
            string memory tag,
            bytes32 evidenceRoot,
            uint64 timestamp,
            bytes32 eventLeaf
        )
    {
        Feedback storage f = _feedback[agent][index];
        return (f.client, f.score, f.tag, f.evidenceRoot, f.timestamp, f.eventLeaf);
    }

    /// @notice Aggregate score across all feedback entries for an agent.
    function aggregateScore(address agent) external view returns (int256 total, uint256 count) {
        Feedback[] storage entries = _feedback[agent];
        count = entries.length;
        for (uint256 i = 0; i < count; i++) {
            total += entries[i].score;
        }
    }
}
