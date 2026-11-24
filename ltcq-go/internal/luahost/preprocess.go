// Package luahost：预处理原版 Lua 资产中的 --# 指令。
//
// 原版把 C 风格预处理指令藏在 Lua 行注释里（--#include / --#pragma once 等），
// 由自研 C++ 宿主在加载前展开。gopher-lua 不认识这些指令，需先预处理。
package luahost

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// preprocDirective 匹配 --# 开头的指令行，允许 -- 后有空格、行尾注释。
// 捕获组：1=指令名(小写)，2=参数。
var preprocDirective = regexp.MustCompile(`^\s*--\s*#([a-zA-Z]+)\s*(.*?)\s*$`)

// includeRe 解析 #include "path" [once] 的路径部分。
var includeRe = regexp.MustCompile(`^"([^"]+)"`)

// Preprocessor 展开 --# 指令。
type Preprocessor struct {
	root         string            // data 根目录，include 相对它解析
	ostype       string            // 用于 #if OSTYPE == WIN
	defines      map[string]string // 已定义宏
	onceFiles    map[string]bool   // #pragma once / include once 记录
	guardStack   []bool            // 条件编译栈：当前分支是否启用
	includeStack []string          // 防循环 include
}

// NewPreprocessor 创建预处理器。root 为 gameworld/data 目录。
func NewPreprocessor(root, ostype string) *Preprocessor {
	return &Preprocessor{
		root:       root,
		ostype:     ostype,
		defines:    map[string]string{},
		onceFiles:  map[string]bool{},
		guardStack: []bool{true},
	}
}

// Process 读取并预处理文件，返回可直接交给 Lua 编译的源码。
func (p *Preprocessor) Process(path string) (string, error) {
	abs, err := p.resolve(path)
	if err != nil {
		return "", err
	}
	return p.processFile(abs)
}

func (p *Preprocessor) processFile(abs string) (string, error) {
	// 防循环 include
	for _, s := range p.includeStack {
		if s == abs {
			return "", nil // 循环 include：跳过
		}
	}
	p.includeStack = append(p.includeStack, abs)
	defer func() { p.includeStack = p.includeStack[:len(p.includeStack)-1] }()

	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("读取 %s: %w", abs, err)
	}
	// 原版资产有 GBK/UTF-8 混合；Go 字符串按字节处理，Lua 解析器按 UTF-8 读，
	// 这里不做转码（中文出现在字符串/注释里，gopher-lua 按字节接受）。
	lines := strings.Split(string(data), "\n")

	var out []string
	for _, line := range lines {
		m := preprocDirective.FindStringSubmatch(line)
		if m == nil {
			// 非指令行：仅在当前条件分支启用时输出
			if p.active() {
				out = append(out, line)
			}
			continue
		}
		directive := strings.ToLower(m[1])
		arg := strings.TrimSpace(m[2])
		if err := p.handleDirective(directive, arg, abs, &out); err != nil {
			return "", err
		}
	}
	return strings.Join(out, "\n"), nil
}

func (p *Preprocessor) handleDirective(dir, arg, curFile string, out *[]string) error {
	switch dir {
	case "include":
		return p.handleInclude(arg, out)
	case "pragma":
		if strings.EqualFold(arg, "once") {
			p.onceFiles[curFile] = true
		}
	case "ifndef":
		// 头守卫：若已定义则跳过，直到 #endif
		_, defined := p.defines[arg]
		p.guardStack = append(p.guardStack, p.active() && !defined)
	case "ifdef":
		_, defined := p.defines[arg]
		p.guardStack = append(p.guardStack, p.active() && defined)
	case "if":
		val, ok := evalIf(arg, p.defines, p.ostype)
		p.guardStack = append(p.guardStack, p.active() && ok && val)
	case "define":
		if p.active() {
			parts := strings.Fields(arg)
			if len(parts) >= 2 {
				p.defines[parts[0]] = parts[1]
			} else if len(parts) == 1 {
				p.defines[parts[0]] = "1"
			}
		}
	case "endif":
		if len(p.guardStack) > 1 {
			p.guardStack = p.guardStack[:len(p.guardStack)-1]
		}
	case "else":
		// 最小集未使用；保留扩展位（翻转当前层）
		if len(p.guardStack) > 1 {
			parent := p.guardStack[len(p.guardStack)-2]
			top := p.guardStack[len(p.guardStack)-1]
			p.guardStack[len(p.guardStack)-1] = parent && !top
		}
	default:
		// 未知指令：忽略
	}
	return nil
}

func (p *Preprocessor) handleInclude(arg string, out *[]string) error {
	m := includeRe.FindStringSubmatch(arg)
	if m == nil {
		return fmt.Errorf("#include 路径解析失败: %q", arg)
	}
	incPath := m[1]
	once := strings.Contains(strings.ToLower(arg[len(m[0]):]), "once")
	// .config 序列的逗号后缀：include 后保留逗号由调用方拼接，这里剥离
	trailingComma := strings.HasSuffix(strings.TrimSpace(arg[len(m[0]):]), ",")

	abs, err := p.resolve(incPath)
	if err != nil {
		// include 文件缺失：原版会报错；预研阶段输出注释标记并继续
		if p.active() {
			*out = append(*out, fmt.Sprintf("-- [luahost] include not found: %s", incPath))
		}
		return nil
	}
	if p.onceFiles[abs] {
		// 已标记 once（显式 once 或 #pragma once），重复 include 跳过
		return nil
	}
	if !p.active() {
		return nil
	}
	if once {
		p.onceFiles[abs] = true
	}

	expanded, err := p.processFile(abs)
	if err != nil {
		return err
	}
	*out = append(*out, expanded)
	if trailingComma {
		*out = append(*out, ",")
	}
	return nil
}

func (p *Preprocessor) active() bool {
	return p.guardStack[len(p.guardStack)-1]
}

// resolve 把 include 路径解析为绝对路径：支持 / 与 \、大小写不敏感、data/ 前缀。
func (p *Preprocessor) resolve(path string) (string, error) {
	norm := strings.ReplaceAll(path, "\\", string(filepath.Separator))
	if filepath.IsAbs(norm) {
		return norm, nil
	}
	// 尝试直接相对 root
	candidate := filepath.Join(p.root, norm)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	// 尝试去掉 data/ 前缀（include 路径常以 data/ 开头）
	clean := strings.TrimPrefix(strings.ReplaceAll(norm, "/", string(filepath.Separator)), "data"+string(filepath.Separator))
	candidate = filepath.Join(p.root, clean)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	// 大小写不敏感查找：遍历 root 找匹配相对路径
	found, err := findCaseInsensitive(p.root, clean)
	if err == nil {
		return found, nil
	}
	return "", fmt.Errorf("include 路径未找到: %s", path)
}

// findCaseInsensitive 在 root 下按大小写不敏感匹配相对路径。
func findCaseInsensitive(root, rel string) (string, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	cur := root
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return "", err
		}
		matched := ""
		for _, e := range entries {
			if strings.EqualFold(e.Name(), part) {
				matched = e.Name()
				break
			}
		}
		if matched == "" {
			return "", fmt.Errorf("未找到 %s in %s", part, cur)
		}
		cur = filepath.Join(cur, matched)
	}
	return cur, nil
}

// evalIf 求值 #if 表达式（最小集：OSTYPE == WIN、宏比较、宏存在）。
func evalIf(arg string, defines map[string]string, ostype string) (bool, bool) {
	// OSTYPE == WIN
	if m := regexp.MustCompile(`OSTYPE\s*==\s*(\w+)`).FindStringSubmatch(arg); m != nil {
		return strings.EqualFold(ostype, m[1]), true
	}
	// MACRO == value
	if m := regexp.MustCompile(`^(\w+)\s*==\s*(\w+)$`).FindStringSubmatch(arg); m != nil {
		v, ok := defines[m[1]]
		return ok && strings.EqualFold(v, m[2]), true
	}
	// 裸宏名：存在即为真
	if _, ok := defines[arg]; ok {
		return true, true
	}
	return false, true
}
