# Henri

Named after Henri Cartier-Bresson [wiki](https://en.wikipedia.org/wiki/Henri_Cartier-Bresson)

A small simple utility that uses a LLaVA multimodal LLM server to classify a library of photos and stores the descriptions in a SQLite DB. That's it. It's a research project for me, but learn from it if you want.

## Usage

Two steps to using. First step, only needs to be done once, walk a photo library looking for JPEGs

```
$ go run ./cmd/henri --library ~/Photos/my_photo_library
Found 21397 images on disk
Added 21397 new images
```

Before starting the second step, which is the photo description step, you should start the LLaVA server.

```
(In another terminal window)
$ cd llavafile
$ ./llava-v1.5-7b-q4.llamafile   # Starts a server listening on http://localhost:8080
```

Once the server is running proceed with the second step. This will take a while (read days)

```
$ go run ./cmd/henri
21397 images to process
Processing 0/21397 <1310: 164E5EBC-F2F5-4B28-A78F-0803857336BE_1_105_c.jpeg> okay, 20 secs
Processing 1/21397 <1311: 16502306-C779-4C32-9E65-5AED005AD9D1_1_105_c.jpeg> okay, 20 secs
Processing 2/21397 <1312: 1650BB2D-5A87-4BDC-A155-9D5BC9BBE8BF_1_105_c.jpeg> okay, 20 secs
Processing 3/21397 <1313: 165AC2AF-E0B4-4EBA-8F94-1B1E03792314_1_105_c.jpeg> okay, 20 secs
Processing 4/21397 <1314: 166BD395-0788-409F-9903-91DFF8B45279_1_105_c.jpeg> okay, 19 secs
Processing 5/21397 <1315: 166BEFB4-EBF0-4C01-83E8-2F1F2DB9C89D_1_105_c.jpeg> okay, 14 secs
Processing 6/21397 <1316: 1675A394-70C3-408D-B63B-05063458483A_1_105_c.jpeg> okay, 20 secs
Processing 7/21397 <1317: 167CD173-33BA-4763-9991-976C42971280_1_105_c.jpeg> okay, 17 secs
Processing 8/21397 <1318: 16812FFB-7428-4260-B6C0-F7E78C69B6E2_1_105_c.jpeg> okay, 14 secs
Processing 9/21397 <1319: 1683DD25-6E33-4754-8C9F-63CB78945DCD_1_105_c.jpeg> okay, 20 secs
Processing 10/21397 <1320: 1684EB8A-3439-47F7-8EFD-F3C5EC9B3536_1_105_c.jpeg> okay, 17 secs
Processing 11/21397 <1321: 16870CC5-21F2-410B-A132-442AA2144E9A_1_105_c.jpeg> okay, 15 secs
Processing 12/21397 <1322: 168ABFBE-4249-4B18-A592-60C331EFC97F_1_105_c.jpeg> okay, 18 secs
Processing 13/21397 <1323: 168B6E18-08B3-467C-92E8-B60A69BF669F_1_102_o.jpeg> okay, 21 secs
Processing 14/21397 <1324: 168B6E18-08B3-467C-92E8-B60A69BF669F_1_105_c.jpeg> okay, 18 secs
Processing 15/21397 <1325: 16933634-DA58-4A8B-9BB9-167E14D7774F_1_105_c.jpeg> okay, 19 secs
Processing 16/21397 <1326: 1694A0F2-D116-4A4E-AF3D-BB40179BB0AC_1_102_o.jpeg> okay, 15 secs
...
```

The utility assumes that the LLaVA server is available at `http://localhost:8080` and has a `POST /completion` endpoint that accepts `JSON` requests. You can use the `--server` option to specify a new server host and port.

## LLaVA

You will need to install and run the LLaVA model for yourself. For simplicity I used the [llamafile](https://github.com/Mozilla-Ocho/llamafile) variant, which is a single executable that embeds llama.cpp running as a server and the GGUF model parameters. Variant I [used](https://huggingface.co/jartine/llava-v1.5-7B-GGUF/blob/main/llava-v1.5-7b-q4.llamafile).

## TODOs

- Proper database schema migration
- Switch timestamps in DB to integers? e.g. `approved_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now'))`
- Thumbnail generation?
