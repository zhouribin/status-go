package main

import (
	"fmt"
	"strings"
)

// packageNode is one node in a recursive package
// structure.
type packageNode struct {
	name     string
	children packageNodes
}

// IndentString returns a node as string with an indentation.
func (n packageNode) IndentString(level int) string {
	tabs := ""
	for i := 0; i < level; i++ {
		tabs += "\t"
	}
	s := fmt.Sprintf("%sNAME %s\n", tabs, n.name)
	for _, child := range n.children {
		s += fmt.Sprintf("%v", child.IndentString(level+1))
	}
	return s
}

// String implements fmt.Stringer.
func (n packageNode) String() string {
	return n.IndentString(0)
}

// packageNodes is a list of package nodes.
type packageNodes []*packageNode

// splitPackages splits the external packages into a list of
// trees if the parts like hosts,
func splitPackages(packageMap importsToFiles) packageNodes {
	pns := packageNodes{}
	for packageName := range packageMap {
		parts := strings.Split(packageName, "/")
		splitPackage(parts, &pns)
	}
	return pns
}

// splitPackage splits the parts of the package names and
// stores them
func splitPackage(parts []string, pns *packageNodes) {
	if len(parts) == 0 {
		// Done.
		return
	}
	head := parts[0]
	tail := parts[1:]
	// Look if head already exists.
	for _, node := range *pns {
		if node.name == head {
			splitPackage(tail, &node.children)
			return
		}
	}
	// No, so append new one.
	node := &packageNode{
		name:     head,
		children: packageNodes{},
	}
	*pns = append(*pns, node)
	splitPackage(tail, &node.children)
}
