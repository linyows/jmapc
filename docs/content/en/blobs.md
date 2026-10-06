# Blobs

Attachments are not transferred through the API endpoint. They are uploaded and
downloaded over plain HTTP, at the URLs the session advertises, and the runtime
handles both:

```go
info, err := c.Upload(ctx, accountID, "application/pdf", file)
// info.BlobID now goes into an Email/set that attaches it.

blob, err := c.Download(ctx, accountID, part.BlobID, &jmapc.DownloadOptions{
	Name: *part.Name,
	Type: part.Type,
})
defer blob.Close()
```

Both stream. `Upload` reads from an `io.Reader` and `Download` returns an
`io.ReadCloser`, so an attachment larger than memory is copied from a file to
the server, and from the server to a file, without being held in either
direction. An upload larger than the server's `maxSizeUpload` fails before it
is sent.

`From` and `Length` fetch part of a blob, which is how a download interrupted
part way is resumed:

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
...
blob.Range // the part that came back: 4096-8191 of 8192
```

They are sent as an HTTP `Range` header. JMAP defines none for the download
endpoint, so a server is free to ignore it and answer with the whole blob.
HTTP does not let a server that answers with part of it answer with a part that
does not fit what was asked, but one that gets ranges wrong may: a part that
starts somewhere else, ends past where it was asked to, ends sooner while the
blob goes on, or does not say where it lies. Either way the download fails rather than returning content the
caller would write at the wrong offset, or handing the caller less than it
asked for. A part that ends sooner because the blob ends first is returned, and
`blob.Range` says how much came back. `IsRangeIgnored` reports the failure,
which asking again does not change, so a download that cannot be resumed starts
over:

```go
blob, err := c.Download(ctx, accountID, blobID, &jmapc.DownloadOptions{From: written})
if jmapc.IsRangeIgnored(err) {
	// The server does not do ranges. Download the whole blob instead.
	written = 0
	blob, err = c.Download(ctx, accountID, blobID, nil)
}
```

A server offering `urn:ietf:params:jmap:blob` can also create and read blobs
through the API, which the endpoints cannot: `Blob/upload` puts a blob in the
same request as the call that uses it, so the id never comes back to the client
in between.
