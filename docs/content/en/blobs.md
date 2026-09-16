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
endpoint, so a server is free to ignore it and answer with the whole blob;
where that happens the download fails rather than returning content the caller
would write at the wrong offset.

A server offering `urn:ietf:params:jmap:blob` can also create and read blobs
through the API, which the endpoints cannot: `Blob/upload` puts a blob in the
same request as the call that uses it, so the id never comes back to the client
in between.
