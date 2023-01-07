package main

import (
	"testing"

	"github.com/samber/lo"
)

func TestMergeUrl(t *testing.T) {
	var cases = []lo.Tuple3[string, string, string]{
		{
			A: "https://a.com",
			B: "https://b.com",
			C: "https://b.com",
		},
		{
			A: "https://a.com",
			B: "/b.com",
			C: "https://a.com/b.com",
		},
		{
			A: "https://a.com/",
			B: "/b.com",
			C: "https://a.com/b.com",
		},
		{
			A: "https://a.com/a",
			B: "b.com",
			C: "https://a.com/a/b.com",
		},
		{
			A: "https://a.com/a",
			B: "http://b.com",
			C: "http://b.com",
		},
		{
			A: "https://a.com/a",
			B: "//b.com",
			C: "https://b.com",
		},
		{
			A: "https://a.com/a.html",
			B: "/b.html?a=b",
			C: "https://a.com/b.html?a=b",
		},
		{
			A: "https://a.com/a",
			B: "/b.com",
			C: "https://a.com/b.com",
		},
	}

	for _, c := range cases {
		res := mergeUrl(c.A, c.B)
		if res != c.C {
			t.Errorf("mergeUrl(%s, %s) %s != %s", c.A, c.B, res, c.C)
		}
	}
}
