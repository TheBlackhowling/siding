module github.com/TheBlackhowling/siding/testdata/externalconsumer

go 1.25

require github.com/TheBlackhowling/siding v0.0.0

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace github.com/TheBlackhowling/siding => ../..
