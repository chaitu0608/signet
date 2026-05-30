package chain

import (
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// BuildMerkleTree returns root and per-leaf proofs for sorted leaf hashes (hex with 0x).
func BuildMerkleTree(leafHex []string) (root string, proofs map[string][]string) {
	if len(leafHex) == 0 {
		return "", map[string][]string{}
	}
	leaves := make([][32]byte, len(leafHex))
	for i, h := range leafHex {
		leaves[i] = common.HexToHash(h)
	}
	sort.Slice(leaves, func(i, j int) bool {
		return bytesCompare(leaves[i][:], leaves[j][:]) < 0
	})

	proofs = make(map[string][]string)
	if len(leaves) == 1 {
		root = "0x" + common.Bytes2Hex(leaves[0][:])
		proofs[root] = nil
		proofs[leafHex[0]] = nil
		return root, proofs
	}

	// Build tree levels and capture proofs.
	level := leaves
	indexMap := make(map[[32]byte]int)
	for i, l := range level {
		indexMap[l] = i
	}

	leafProofs := make([][][32]byte, len(level))
	for i := range leafProofs {
		leafProofs[i] = nil
	}

	for len(level) > 1 {
		var next [][32]byte
		for i := 0; i < len(level); i += 2 {
			var right [32]byte
			if i+1 < len(level) {
				right = level[i+1]
			} else {
				right = level[i]
			}
			left := level[i]
			parent := hashPair(left, right)

			// sibling for left
			idx := indexMap[left]
			if idx < len(leafProofs) {
				leafProofs[idx] = append(leafProofs[idx], right)
			}
			// sibling for right if exists
			if i+1 < len(level) {
				idx2 := indexMap[right]
				if idx2 < len(leafProofs) {
					leafProofs[idx2] = append(leafProofs[idx2], left)
				}
			}

			next = append(next, parent)
		}
		level = next
	}

	root = "0x" + common.Bytes2Hex(level[0][:])

	for i, l := range leaves {
		h := "0x" + common.Bytes2Hex(l[:])
		var proofHex []string
		for _, s := range leafProofs[i] {
			proofHex = append(proofHex, "0x"+common.Bytes2Hex(s[:]))
		}
		proofs[h] = proofHex
	}
	return root, proofs
}

func hashPair(a, b [32]byte) [32]byte {
	if bytesCompare(a[:], b[:]) > 0 {
		a, b = b, a
	}
	return crypto.Keccak256Hash(append(a[:], b[:]...))
}

func bytesCompare(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return 0
}

// VerifyProof checks a Merkle proof against root.
func VerifyProof(rootHex, leafHex string, proof []string) bool {
	leaf := common.HexToHash(leafHex)
	computed := leaf
	for _, p := range proof {
		sib := common.HexToHash(p)
		computed = hashPair(computed, sib)
	}
	return computed.Hex() == common.HexToHash(rootHex).Hex()
}
