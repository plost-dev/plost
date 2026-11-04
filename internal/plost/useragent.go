package plost

import "strings"

// A browser is named by the first rule whose marker its User-Agent contains. In-app browsers come first (WeChat's
// says MQQBrowser and Chrome too), then vendor browsers built on Chromium or WebKit, then the engines' own browsers.
// The names match the icons in frontend/src/components/icons.tsx.
var browserRules = []struct {
	name    string
	markers []string
}{
	{"WeCom", []string{"wxwork/"}},
	{"WeChat", []string{"MicroMessenger/"}},
	{"DingTalk", []string{"DingTalk/"}},
	{"Feishu", []string{"Lark/", "Feishu/"}},
	{"Alipay", []string{"AlipayClient/"}},
	{"Douyin", []string{"aweme", "BytedanceWebview"}},
	{"Weibo", []string{"Weibo (", "__weibo__"}},
	{"QQ", []string{" QQ/"}},
	{"Instagram", []string{"Instagram "}},
	{"Facebook", []string{"FBAN/", "FBAV/", "FB_IAB/"}},
	{"LINE", []string{" Line/"}},
	{"Quark", []string{"Quark/"}},
	{"UC Browser", []string{"UCBrowser/", "UCWEB", "UBrowser/"}},
	{"QQ Browser", []string{"MQQBrowser/", "QQBrowser/"}},
	{"Baidu", []string{"baiduboxapp/", "baidubrowser/", "BIDUBrowser"}},
	{"Sogou Browser", []string{"SogouMobileBrowser/", "SE 2.X MetaSr"}},
	{"360 Browser", []string{"QihooBrowser/", "QHBrowser/", "360SE", "360EE"}},
	{"2345 Browser", []string{"Mb2345Browser/", "2345Explorer/"}},
	{"Huawei Browser", []string{"HuaweiBrowser/", "HBPC/"}},
	{"MIUI Browser", []string{"MiuiBrowser/", "XiaoMi/MiuiBrowser"}},
	{"vivo Browser", []string{"VivoBrowser/", "vivoBrowser/"}},
	{"OPPO Browser", []string{"HeyTapBrowser/", "OppoBrowser/"}},
	{"Samsung Internet", []string{"SamsungBrowser/"}},
	{"Yandex Browser", []string{"YaBrowser/", "YaSearchBrowser/"}},
	{"DuckDuckGo", []string{"DuckDuckGo/", "Ddg/"}},
	{"Vivaldi", []string{"Vivaldi/"}},
	{"Amazon Silk", []string{"Silk/"}},
	{"Edge", []string{"Edg/", "EdgA/", "EdgiOS/", "Edge/"}},
	{"Opera", []string{"OPR/", "OPT/", "OPiOS/", "Opera Mini", "Opera/"}},
	{"Firefox", []string{"Firefox/", "FxiOS/"}},
	{"Internet Explorer", []string{"MSIE ", "Trident/"}},
}

// Systems, most specific first: HarmonyOS and Fire OS also say Android, and an iPad asking for the desktop site says
// Macintosh but keeps the Mobile/ token of iOS.
func parseOS(ua string) string {
	has := func(s string) bool { return strings.Contains(ua, s) }
	switch {
	case has("OpenHarmony") || has("HarmonyOS"):
		return "HarmonyOS"
	case has("Windows Phone"):
		return "Windows Phone"
	case has("KAIOS") || has("KaiOS"):
		return "KaiOS"
	case has("Tizen"):
		return "Tizen"
	case has("Silk/") || has("; KF"):
		return "Fire OS"
	case has("Android"):
		return "Android"
	case has("iPad") || (has("Macintosh") && has(" Mobile/")):
		return "iPadOS"
	case has("iPhone") || has("iPod"):
		return "iOS"
	case has("Windows"):
		return "Windows"
	case has("CrOS"):
		return "Chrome OS"
	case has("Macintosh") || has("Mac OS X"):
		return "macOS"
	case has("PlayStation"):
		return "PlayStation"
	case has("FreeBSD"):
		return "FreeBSD"
	case has("Ubuntu"):
		return "Ubuntu"
	case has("Fedora"):
		return "Fedora"
	case has("Linux") || has("X11"):
		return "Linux"
	}
	return "Other"
}

func parseUA(ua string) (browser, osName, device string) {
	has := func(s string) bool { return strings.Contains(ua, s) }
	osName = parseOS(ua)
	browser = "Other"
	for _, rule := range browserRules {
		for _, marker := range rule.markers {
			if has(marker) {
				browser = rule.name
				break
			}
		}
		if browser != "Other" {
			break
		}
	}
	if browser == "Other" {
		apple := osName == "iOS" || osName == "iPadOS"
		switch {
		case has("CriOS/"):
			browser = "Chrome"
		case has("; wv)") || (has("Version/4.0") && has("Chrome/")):
			browser = "Chromium WebView"
		case has("Chrome/") || has("Chromium/"):
			browser = "Chrome"
		case apple && has("AppleWebKit") && !has("Safari/"):
			browser = "iOS WebView"
		case has("Safari/"):
			browser = "Safari"
		}
	}
	device = "Desktop"
	switch {
	case osName == "iPadOS" || has("Tablet") || osName == "Fire OS" || (osName == "Android" && !has("Mobile")):
		device = "Tablet"
	case has("Mobile") || has("iPhone") || has("iPod") || has("Phone;") || osName == "Windows Phone" || osName == "KaiOS":
		device = "Mobile"
	}
	return
}
