# tumblr-scraper

This project was created as a black box, from scratch reimplementation of [Liru/tumblr-downloader](https://github.com/Liru/tumblr-downloader), recreating and improving upon its features.

## Features

* Downloads all photos and videos of a blog, including those inlined into posts
* Automatically stops scraping a blog where it left off the last time
* Allows filtering out reblogs
* Uses Tumblr's v2 API, which is more robust and significantly faster
* Simulates Tumblr's private API to even scrape private blogs if needed
* All downloads are parallelized

## TODOs

* Documentation (up until now this strictly has been a private project)
* Crawling of >5000 posts per day will lead to rate limiting
* Continuing a previously failed crawl/scrape is not supported<br>
  Setting the `before` field in the config allows you to scrape backwards starting at a date in the past.<br>
  That way you can manually, iteratively scrape a huge blog in "sane" chunks (e.g. first everything before 2014, then 2015, 2016, ...).
* Support for `youtube-dl` would be nice
