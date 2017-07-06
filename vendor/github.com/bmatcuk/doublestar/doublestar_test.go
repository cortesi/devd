// This file is mostly copied from Go's path/match_test.go

package doublestar

import "testing"

type MatchTest struct {
  pattern, s string
  match      bool
  err        error
  testGlob   bool
}

var matchTests = []MatchTest{
  {"abc", "abc", true, nil, true},
  {"*", "abc", true, nil, true},
  {"*c", "abc", true, nil, true},
  {"a*", "a", true, nil, true},
  {"a*", "abc", true, nil, true},
  {"a*", "ab/c", false, nil, true},
  {"a*/b", "abc/b", true, nil, true},
  {"a*/b", "a/c/b", false, nil, true},
  {"a*b*c*d*e*/f", "axbxcxdxe/f", true, nil, true},
  {"a*b*c*d*e*/f", "axbxcxdxexxx/f", true, nil, true},
  {"a*b*c*d*e*/f", "axbxcxdxe/xxx/f", false, nil, true},
  {"a*b*c*d*e*/f", "axbxcxdxexxx/fff", false, nil, true},
  {"a*b?c*x", "abxbbxdbxebxczzx", true, nil, true},
  {"a*b?c*x", "abxbbxdbxebxczzy", false, nil, true},
  {"ab[c]", "abc", true, nil, true},
  {"ab[b-d]", "abc", true, nil, true},
  {"ab[e-g]", "abc", false, nil, true},
  {"ab[^c]", "abc", false, nil, true},
  {"ab[^b-d]", "abc", false, nil, true},
  {"ab[^e-g]", "abc", true, nil, true},
  {"a\\*b", "a*b", true, nil, true},
  {"a\\*b", "ab", false, nil, true},
  {"a?b", "a☺b", true, nil, true},
  {"a[^a]b", "a☺b", true, nil, true},
  {"a???b", "a☺b", false, nil, true},
  {"a[^a][^a][^a]b", "a☺b", false, nil, true},
  {"[a-ζ]*", "α", true, nil, true},
  {"*[a-ζ]", "A", false, nil, true},
  {"a?b", "a/b", false, nil, true},
  {"a*b", "a/b", false, nil, true},
  {"[\\]a]", "]", true, nil, true},
  {"[\\-]", "-", true, nil, true},
  {"[x\\-]", "x", true, nil, true},
  {"[x\\-]", "-", true, nil, true},
  {"[x\\-]", "z", false, nil, true},
  {"[\\-x]", "x", true, nil, true},
  {"[\\-x]", "-", true, nil, true},
  {"[\\-x]", "a", false, nil, true},
  {"[]a]", "]", false, ErrBadPattern, true},
  {"[-]", "-", false, ErrBadPattern, true},
  {"[x-]", "x", false, ErrBadPattern, true},
  {"[x-]", "-", false, ErrBadPattern, true},
  {"[x-]", "z", false, ErrBadPattern, true},
  {"[-x]", "x", false, ErrBadPattern, true},
  {"[-x]", "-", false, ErrBadPattern, true},
  {"[-x]", "a", false, ErrBadPattern, true},
  {"\\", "a", false, ErrBadPattern, true},
  {"[a-b-c]", "a", false, ErrBadPattern, true},
  {"[", "a", false, ErrBadPattern, true},
  {"[^", "a", false, ErrBadPattern, true},
  {"[^bc", "a", false, ErrBadPattern, true},
  {"a[", "a", false, nil, false},
  {"a[", "ab", false, ErrBadPattern, true},
  {"*x", "xxx", true, nil, true},
  {"a/**", "a", false, nil, true},
  {"a/**", "a/b", true, nil, true},
  {"a/**", "a/b/c", true, nil, true},
  {"**/c", "c", true, nil, true},
  {"**/c", "b/c", true, nil, true},
  {"**/c", "a/b/c", true, nil, true},
  {"a/**/b", "a/b", true, nil, true},
  {"a/**/c", "a/b/c", true, nil, true},
  {"a/**/d", "a/b/c/d", true, nil, true},
  {"a/\\**", "a/b/c", false, nil, true},
  {"a/\\**", "a/*", true, nil, true},
  {"ab{c,d}", "abc", true, nil, true},
  {"ab{c,d,*}", "abcde", true, nil, true},
  {"ab{c,d}[", "abcd", false, ErrBadPattern, true},
}

func TestMatch(t *testing.T) {
  for idx, tt := range matchTests {
    testMatchWith(t, idx, tt)
  }
}

func testMatchWith(t *testing.T, idx int, tt MatchTest) {
  defer func() {
    if r := recover(); r != nil {
      t.Errorf("#%v. Match(%#q, %#q) panicked: %#v", idx, tt.pattern, tt.s, r)
    }
  }()

  ok, err := Match(tt.pattern, tt.s)
  if ok != tt.match || err != tt.err {
    t.Errorf("#%v. Match(%#q, %#q) = %v, %v want %v, %v", idx, tt.pattern, tt.s, ok, err, tt.match, tt.err)
  }
}

func TestGlob(t *testing.T) {
  for idx, tt := range matchTests {
    if tt.testGlob {
      testGlobWith(t, idx, tt)
    }
  }
}

func testGlobWith(t *testing.T, idx int, tt MatchTest) {
  defer func() {
    if r := recover(); r != nil {
      t.Errorf("#%v. Glob(%#q) panicked: %#v", idx, tt.pattern, r)
    }
  }()

  matches, err := Glob("test/" + tt.pattern)
  if inSlice("test/" + tt.s, matches) != tt.match {
    if tt.match {
      t.Errorf("#%v. Glob(%#q) = %#v - doesn't contain %v, but should", idx, tt.pattern, matches, tt.s)
    } else {
      t.Errorf("#%v. Glob(%#q) = %#v - contains %v, but shouldn't", idx, tt.pattern, matches, tt.s)
    }
  }
  if err != tt.err {
    t.Errorf("#%v. Glob(%#q) has error %v, but should be %v", idx, tt.pattern, err, tt.err)
  }
}

func inSlice(s string, a []string) bool {
  for _, i := range a {
    if i == s { return true }
  }
  return false
}

