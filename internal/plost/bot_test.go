package plost

import "testing"

func TestIsBotKeepsPeopleAndDropsCrawlers(t *testing.T) {
	people := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36 Edg/142.0.0.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 13; CUBOT KingKong 9 Build/TP1A) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 12; CUBOT_X50) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Mobile Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.50(0x1800323a) NetType/WIFI Language/zh_CN",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 [FBAN/FBIOS;FBAV/470.0.0.0]",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36 Instagram 350.0.0.0",
		"Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36 [Pinterest/Android]",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 YaBrowser/24.10.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 OPR/115.0.0.0",
	}
	for _, ua := range people {
		if isBot(ua) {
			t.Errorf("a person's browser was taken for a bot: %s", ua)
		}
	}
	bots := []string{
		"",
		"   ",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm) Chrome/116.0.1938.76 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Linux; Android 11; moto g power (2022)) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Mobile Safari/537.36 Chrome-Lighthouse",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
		"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)",
		"Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)",
		"Mozilla/5.0 (Linux; Android 5.0) AppleWebKit/537.36 (KHTML, like Gecko) Mobile Safari/537.36 (compatible; Bytespider; spider-feedback@bytedance.com)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
		"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko); compatible; ClaudeBot/1.0; +claudebot@anthropic.com",
		"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)",
		"Mozilla/5.0 (compatible; SemrushBot/7~bl; +http://www.semrush.com/bot.html)",
		"Mozilla/5.0 (compatible; UptimeRobot/2.0; http://www.uptimerobot.com/)",
		"Pingdom.com_bot_version_1.4_(http://www.pingdom.com/)",
		"WhatsApp/2.23.20.0 A",
		"TelegramBot (like TwitterBot)",
		"python-requests/2.32.3",
		"curl/8.7.1",
		"Go-http-client/2.0",
		"Scrapy/2.11.2 (+https://scrapy.org)",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 (compatible; PerplexityBot/1.0)",
	}
	for _, ua := range bots {
		if !isBot(ua) {
			t.Errorf("a bot got through: %q", ua)
		}
	}
}
