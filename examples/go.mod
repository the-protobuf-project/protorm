module github.com/the-protobuf-project/protorm/examples

go 1.26.4

// replace points at the repo root so go.work resolves the local protorm module
// without publishing it to the registry first.
replace github.com/the-protobuf-project/protorm => ../
