#!/bin/sh
OUT=..
go build -o codegen &&
  ./codegen -cout $OUT/rule_gen.go -tout $OUT/rule_gen_test.go && \
  gofmt -w=true $OUT/rule_gen.go && \
  gofmt -w=true $OUT/rule_gen_test.go && \
  rm codegen
