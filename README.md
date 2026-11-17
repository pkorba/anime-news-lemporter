# Anime News Lemporter

This is a Lemmy bot that reposts anime news (announcements, key visuals, etc.) to a chosen 
community. It uses Lemmy APIv3 to communicate with Lemmy, and sources data from Tenrai API.

Before posting, the bot checks whether anyone else has already created a post within the last week 
in the given community that includes the title of the anime and concerns an announcement. If such 
a post is found, the bot does not create a new one. Posts created by the bot's own account and 
episode threads (tagged with [Episode]) are not taken into account.

## Installation

Download the binary, run it as cronjob or with systemd timer.

## Configuration

You need to launch the program with these environment variables.

|env_var|description|
|-|-|
|`LEMMY_USERNAME`|Lemmy account username (without *@instance.tld* part)|
|`LEMMY_PASSWORD`|Lemmy account password|
|`LEMMY_INSTANCE`|Lemmy instance (e.g., *ani.social*) where the bot account is registered|
|`LEMMY_COMMUNITY_NAME`|The name of the community where the bot will post|
|`WORKING_DIR`|Directory where database, log, and blacklist files are stored|

It's possible, but entirely optional, to have a `blacklist.txt` file in `WORKING_DIR`. 
Each line in this file is treated as a separate phrase. If that phrase is found in a source's 
title, that source will not be posted to Lemmy. `blacklist.txt` only check against the original 
titles (before replacing Romaji title with its English equivalent), so if you want to exclude 
news by using an anime title, it needs to be Romaji title.
