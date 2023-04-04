package memiavl

import (
	"testing"

	"github.com/cosmos/iavl"
	"github.com/stretchr/testify/require"
)

func TestSnapshotEncodingRoundTrip(t *testing.T) {
	// setup test tree
	tree := NewEmptyTree(0, DefaultPathToWAL)
	for _, changes := range ChangeSets[:len(ChangeSets)-1] {
		_, _, err := tree.ApplyChangeSet(&changes, true)
		require.NoError(t, err)
	}

	snapshotDir := t.TempDir()
	require.NoError(t, tree.WriteSnapshot(snapshotDir, true))

	snapshot, err := OpenSnapshot(snapshotDir)
	require.NoError(t, err)

	tree2 := NewFromSnapshot(snapshot, DefaultPathToWAL)

	require.Equal(t, tree.Version(), tree2.Version())
	require.Equal(t, tree.RootHash(), tree2.RootHash())

	// verify all the node hashes in snapshot
	for i := 0; i < snapshot.nodesLen(); i++ {
		node := snapshot.Node(uint32(i))
		require.Equal(t, node.Hash(), HashNode(node))
	}

	require.NoError(t, snapshot.Close())

	// test modify tree loaded from snapshot
	snapshot, err = OpenSnapshot(snapshotDir)
	require.NoError(t, err)
	tree3 := NewFromSnapshot(snapshot, DefaultPathToWAL)
	hash, v, err := tree3.ApplyChangeSet(&ChangeSets[len(ChangeSets)-1], true)
	require.NoError(t, err)
	require.Equal(t, RefHashes[len(ChangeSets)-1], hash)
	require.Equal(t, len(ChangeSets), int(v))
	require.NoError(t, snapshot.Close())
}

func TestSnapshotExport(t *testing.T) {
	expNodes := []*iavl.ExportNode{
		{Key: []byte("hello"), Value: []byte("world1"), Version: 2, Height: 0},
		{Key: []byte("hello1"), Value: []byte("world1"), Version: 2, Height: 0},
		{Key: []byte("hello1"), Value: nil, Version: 3, Height: 1},
		{Key: []byte("hello2"), Value: []byte("world1"), Version: 3, Height: 0},
		{Key: []byte("hello3"), Value: []byte("world1"), Version: 3, Height: 0},
		{Key: []byte("hello3"), Value: nil, Version: 3, Height: 1},
		{Key: []byte("hello2"), Value: nil, Version: 3, Height: 2},
	}

	// setup test tree
	tree := NewEmptyTree(0, DefaultPathToWAL)
	for _, changes := range ChangeSets[:3] {
		_, _, err := tree.ApplyChangeSet(&changes, true)
		require.NoError(t, err)
	}

	snapshotDir := t.TempDir()
	require.NoError(t, tree.WriteSnapshot(snapshotDir, true))

	snapshot, err := OpenSnapshot(snapshotDir)
	require.NoError(t, err)

	var nodes []*iavl.ExportNode
	exporter := snapshot.Export()
	for {
		node, err := exporter.Next()
		if err == iavl.ExportDone {
			break
		}
		require.NoError(t, err)
		nodes = append(nodes, node)
	}

	require.Equal(t, expNodes, nodes)
}

func TestSnapshotImportExport(t *testing.T) {
	// setup test tree
	tree := NewEmptyTree(0, DefaultPathToWAL)
	for _, changes := range ChangeSets {
		_, _, err := tree.ApplyChangeSet(&changes, true)
		require.NoError(t, err)
	}

	snapshotDir := t.TempDir()
	require.NoError(t, tree.WriteSnapshot(snapshotDir, true))
	snapshot, err := OpenSnapshot(snapshotDir)
	require.NoError(t, err)

	ch := make(chan *iavl.ExportNode)

	go func() {
		defer close(ch)

		exporter := snapshot.Export()
		for {
			node, err := exporter.Next()
			if err == iavl.ExportDone {
				break
			}
			require.NoError(t, err)
			ch <- node
		}
	}()

	snapshotDir2 := t.TempDir()
	err = Import(snapshotDir2, tree.Version(), ch, true)
	require.NoError(t, err)

	snapshot2, err := OpenSnapshot(snapshotDir2)
	require.NoError(t, err)
	require.Equal(t, snapshot.RootNode().Hash(), snapshot2.RootNode().Hash())

	// verify all the node hashes in snapshot
	for i := 0; i < snapshot2.nodesLen(); i++ {
		node := snapshot2.Node(uint32(i))
		require.Equal(t, node.Hash(), HashNode(node))
	}
}
