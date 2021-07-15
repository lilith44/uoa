package uoa

import (
	`context`
	`fmt`
	`net/url`
	`strings`
)

// 内部接口封装
// 使用模板方法设计模式
type uoaTemplate struct {
	cos uoaInternal
}

func (t *uoaTemplate) Sts(ctx context.Context, path Path, opts ...stsOption) (sts Sts, err error) {
	options := defaultStsOptions()
	for _, opt := range opts {
		opt.applySts(options)
	}

	key := t.key(path, options.environment, options.separator)
	var keys []string
	if 0 != len(options.patterns) {
		keys = make([]string, 0, len(options.patterns))
		for _, pattern := range options.patterns {
			keys = append(keys, fmt.Sprintf("%s%s%s", key, options.separator, pattern))
		}
	} else {
		keys = []string{key}
	}
	switch options.uoaType {
	case TypeCos:
		sts, err = t.cos.sts(ctx, options, keys...)
	}

	return
}

func (t *uoaTemplate) Url(ctx context.Context, path Path, filename string, opts ...urlOption) (uri string, err error) {
	options := defaultUrlOptions()
	for _, opt := range opts {
		opt.applyUrl(options)
	}

	key := t.key(path, options.environment, options.separator)
	var originalURL *url.URL
	switch options.uoaType {
	case TypeCos:
		originalURL, err = t.cos.url(ctx, key, filename, options)
	}
	if nil != err {
		return
	}
	// 解决Golang JSON序列化时的HTML Escape
	uri = t.escape(originalURL)

	return
}

func (t *uoaTemplate) key(path Path, environment string, separator string) (key string) {
	paths := path.Paths()
	if "" != environment {
		paths = append([]string{environment}, paths...)
	}
	key = strings.Join(path.Paths(), separator)

	return
}

func (t *uoaTemplate) escape(originalURL *url.URL) (url string) {
	url = originalURL.String()
	url = strings.Replace(url, "\\u003c", "<", -1)
	url = strings.Replace(url, "\\u003e", ">", -1)
	url = strings.Replace(url, "\\u0026", "&", -1)

	return url
}
