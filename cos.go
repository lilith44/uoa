package uoa

import (
	`context`
	`crypto/tls`
	`encoding/json`
	`fmt`
	`net/http`
	`net/url`
	`strings`
	`sync`
	`time`

	`github.com/storezhang/gox`
	`github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common`
	`github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile`
	sts `github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sts/v20180813`
	`github.com/tencentyun/cos-go-sdk-v5`
)

var _ uoaInternal = (*cosInternal)(nil)

// cosInternal 腾讯云存储
type cosInternal struct {
	clientCache sync.Map
}

func (c *cosInternal) url(ctx context.Context, key string, filename string, options *urlOptions) (url *url.URL, err error) {
	var (
		client      *cos.Client
		getOptions  *cos.ObjectGetOptions
		headRsp     *cos.Response
		contentType string
	)

	if client, err = c.getClient(options.endpoint, options.secret); nil != err {
		return
	}
	// 检查文件是否存在，文件不存在没必要往下继续执行
	if headRsp, err = client.Object.Head(ctx, key, nil); nil != err {
		if rspErr, ok := err.(*cos.ErrorResponse); ok && http.StatusNotFound == rspErr.Response.StatusCode {
			err = nil
		}

		return
	}

	if options.download {
		getOptions = &cos.ObjectGetOptions{
			ResponseContentDisposition: gox.ContentDisposition(filename, gox.ContentDispositionTypeAttachment),
		}
	} else if options.inline {
		if "" != options.contentType {
			contentType = options.contentType
		} else {
			contentType = headRsp.Header.Get(gox.HeaderContentType)
		}
		getOptions = &cos.ObjectGetOptions{
			ResponseContentDisposition: gox.ContentDisposition(filename, gox.ContentDispositionTypeInline),
			ResponseContentType:        contentType,
		}
	}

	// 获取预签名URL
	if url, err = client.Object.GetPresignedURL(
		ctx,
		http.MethodGet,
		key,
		options.secret.Id, options.secret.Key,
		options.expired,
		getOptions,
	); nil != err {
		return
	}

	return
}

func (c *cosInternal) credentials(_ context.Context, options *credentialsOptions, keys ...string) (credentials *credentialsBase, err error) {
	actions := []string{
		// 简单上传
		"name/cos:PutObject",
		// 表单上传、小程序上传
		"name/cos:PostObject",
		// 分块上传：初始化分块操作
		"name/cos:InitiateMultipartUpload",
		// 分块上传：列举进行中的分块上传
		"name/cos:ListMultipartUploads",
		// 分块上传：列举已上传分块操作
		"name/cos:ListParts",
		// 分块上传：上传分块块操作
		"name/cos:UploadPart",
		// 分块上传：完成所有分块上传操作
		"name/cos:CompleteMultipartUpload",
		// 取消分块上传操作
		"name/cos:AbortMultipartUpload",
	}
	region, appId, bucketName := c.parse(options.endpoint)
	resources := make([]string, 0, len(keys))
	for _, key := range keys {
		resources = append(resources, fmt.Sprintf("qcs::cos:%s:uid/%s:%s/%s", region, appId, bucketName, key))
	}
	policy := cosPolicy{
		Version: options.version,
		Statements: []cosStatement{
			{
				Actions:   actions,
				Effect:    "allow",
				Resources: resources,
			},
		},
	}

	var policyBytes []byte
	if policyBytes, err = json.Marshal(policy); nil != err {
		return
	}

	credential := common.NewCredential(options.secret.Id, options.secret.Key)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = options.url
	client, _ := sts.NewClient(credential, region, cpf)

	req := sts.NewGetFederationTokenRequest()
	req.Name = common.StringPtr("cos-credential-go")
	req.Policy = common.StringPtr(string(policyBytes))
	req.DurationSeconds = common.Uint64Ptr(uint64(options.expired / time.Second))

	var rsp *sts.GetFederationTokenResponse
	if rsp, err = client.GetFederationToken(req); nil != err {
		return
	}

	credentials = &credentialsBase{
		Id:      *rsp.Response.Credentials.TmpSecretId,
		Key:     *rsp.Response.Credentials.TmpSecretKey,
		Token:   *rsp.Response.Credentials.Token,
		Expired: time.Unix(int64(*rsp.Response.ExpiredTime), 0),
	}

	return
}

func (c *cosInternal) delete(ctx context.Context, key string, options *deleteOptions) (err error) {
	var client *cos.Client
	if client, err = c.getClient(options.endpoint, options.secret); nil != err {
		return
	}

	opts := make([]*cos.ObjectDeleteOptions, 0, 0)
	if "" != options.version {
		opts = append(opts, &cos.ObjectDeleteOptions{
			VersionId: options.version,
		})
	}
	_, err = client.Object.Delete(ctx, key, opts...)

	return
}

func (c *cosInternal) getClient(baseUrl string, secret gox.Secret) (client *cos.Client, err error) {
	var (
		cache interface{}
		ok    bool
	)

	key := fmt.Sprintf("%s-%s", baseUrl, secret.Id)
	if cache, ok = c.clientCache.Load(key); ok {
		client = cache.(*cos.Client)

		return
	}

	var bucketUrl *url.URL
	if bucketUrl, err = url.Parse(baseUrl); nil != err {
		return
	}

	client = cos.NewClient(&cos.BaseURL{BucketURL: bucketUrl}, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secret.Id,
			SecretKey: secret.Key,
			// nolint:gosec
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		},
	})
	c.clientCache.Store(key, client)

	return
}

func (c *cosInternal) parse(endpoint string) (region string, appId string, bucketName string) {
	endpoint = strings.ReplaceAll(endpoint, "https://", "")
	urls := strings.Split(endpoint, ".")
	region = urls[2]
	bucketName = urls[0]
	appId = strings.Split(urls[0], "-")[1]

	return
}
