package main

import "notes_maker/pkg/core"

func extractOptions(s string) []string {
	return core.ExtractOptions(s)
}

func extractSessionContext(s string) (jsonBody, cleaned string, ok bool) {
	return core.ExtractSessionContext(s)
}

func writeFileBlocks(buf, workDir string, seen map[string]bool) []string {
	return core.WriteFileBlocks(buf, workDir, seen)
}

func displayClean(buf string) string {
	return core.DisplayClean(buf)
}

func writeReceiptNames(s string) []string {
	return core.WriteReceiptNames(s)
}

func splitInProgressFile(buf string) (cleanedChat, name, partial string, ok bool) {
	return core.SplitInProgressFile(buf)
}
