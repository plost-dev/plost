package plost

import "testing"

func TestParseUANamesMobileAndInAppBrowsers(t *testing.T) {
	cases := []struct{ ua, browser, os, device string }{
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36", "Chrome", "Windows", "Desktop"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0", "Edge", "Windows", "Desktop"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", "Safari", "macOS", "Desktop"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", "Safari", "iPadOS", "Tablet"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", "Safari", "iOS", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0.6478.54 Mobile/15E148 Safari/604.1", "Chrome", "iOS", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 MicroMessenger/8.0.49(0x18003133) NetType/WIFI Language/zh_CN", "WeChat", "iOS", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 13; V2227A Build/TP1A.220624.014; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/111.0.5563.116 Mobile Safari/537.36 XWEB/1110017 MMWEBSDK/20230805 MMWEBID/2585 MicroMessenger/8.0.42.2460(0x28002A35) WeChat/arm64 Weixin NetType/WIFI Language/zh_CN ABI/arm64", "WeChat", "Android", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 16_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 QQ/8.9.80.637 V1_IPH_SQ_8.9.80_1_APP_A Pixel/1170 MiniAppEnable SimpleUISwitch/0 StudyMode/0 CurrentMode/0 CurrentFontScale/1.000000 QQTheme/1000 AppId/537170024 Core/WKWebView Device/Apple(iPhone 14) NetType/WIFI QBWebViewType/1 WKType/1", "QQ", "iOS", "Mobile"},
		{"Mozilla/5.0 (Linux; U; Android 12; zh-cn; PFZM10 Build/SP1A.210812.016) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/109.0.5414.86 MQQBrowser/14.9 Mobile Safari/537.36 COVC/046901", "QQ Browser", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; U; Android 12; zh-CN; 2201122C Build/SKQ1.211006.001) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.58 UCBrowser/16.3.8.1290 Mobile Safari/537.36", "UC Browser", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; U; Android 13; zh-CN; 22081212C Build/TKQ1.220829.002) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/100.0.4896.58 Quark/6.11.2.531 Mobile Safari/537.36", "Quark", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; U; Android 13; zh-cn; 2211133C Build/TKQ1.220905.001) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/109.0.5414.118 Mobile Safari/537.36 XiaoMi/MiuiBrowser/18.3.30922", "MIUI Browser", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 12; HarmonyOS; NOH-AN00; HMSCore 6.12.0.302) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.4844.88 HuaweiBrowser/14.0.5.302 Mobile Safari/537.36", "Huawei Browser", "HarmonyOS", "Mobile"},
		{"Mozilla/5.0 (Phone; OpenHarmony 4.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36 ArkWeb/4.1.6.1 Mobile HuaweiBrowser/5.0.4.300", "Huawei Browser", "HarmonyOS", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 13; V2219A; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/87.0.4280.141 Mobile Safari/537.36 VivoBrowser/17.4.10.0", "vivo Browser", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; U; Android 13; zh-cn; PHB110 Build/TP1A.220905.001) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/115.0.5790.168 Mobile Safari/537.36 HeyTapBrowser/40.8.40.1", "OPPO Browser", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36", "Samsung Internet", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36", "Chrome", "Android", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 13; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", "Chrome", "Android", "Tablet"},
		{"Mozilla/5.0 (Linux; Android 13; Pixel 7 Build/TQ3A.230805.001; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/126.0.6478.71 Mobile Safari/537.36", "Chromium WebView", "Android", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", "iOS WebView", "iOS", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_6_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/22G100 Twitter for iPhone/12.40", "iOS WebView", "iOS", "Mobile"},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)", "macOS WebView", "macOS", "Desktop"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 baiduboxapp/13.58.0.10 (Baidu; P2 17.5)", "Baidu", "iOS", "Mobile"},
		{"Mozilla/5.0 (Linux; Android 13; 22041211AC) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36 EdgA/120.0.2210.157", "Edge", "Android", "Mobile"},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/127.0 Mobile/15E148 Safari/605.1.15", "Firefox", "iOS", "Mobile"},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 YaBrowser/24.10.0.0 Safari/537.36", "Yandex Browser", "Windows", "Desktop"},
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 OPR/115.0.0.0", "Opera", "Linux", "Desktop"},
		{"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", "Chrome", "Chrome OS", "Desktop"},
		{"Mozilla/5.0 (Linux; Android 9; KFTRWI) AppleWebKit/537.36 (KHTML, like Gecko) Silk/126.4.1 like Chrome/126.0.6478.153 Safari/537.36", "Amazon Silk", "Fire OS", "Tablet"},
		{"", "Other", "Other", "Desktop"},
	}
	for _, c := range cases {
		browser, os, device := parseUA(c.ua)
		if browser != c.browser || os != c.os || device != c.device {
			t.Errorf("parseUA(%q) = %s / %s / %s, want %s / %s / %s", c.ua, browser, os, device, c.browser, c.os, c.device)
		}
	}
}
