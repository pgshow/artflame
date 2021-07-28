package main

import (
	"encoding/csv"
	"fmt"
	"github.com/antchfx/htmlquery"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gocolly/colly"
)

type product struct {
	Title            string
	Category         string
	Price            string
	ShortDescription string
	LongDescription  string
	Profile          string
	Url              string
	Images_color1    string
	Images_color2    string
	Images_color3    string
	Images_color4    string
}

const (
	//设置常量 分别设置chromedriver.exe的地址和本地调用端口
	seleniumPath = `/usr/bin/chromedriver`
	port         = 9515
)

// chrome浏览器的一些配置
var chromeCaps = chrome.Capabilities{
	Prefs: map[string]interface{}{ // 禁止加载图片，加快渲染速度
		"profile.managed_default_content_settings.images": 2,
	},
	Path: "",
	Args: []string{
		//"--headless",
		//"--start-maximized",
		"--no-sandbox",
		"--user-agent=Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/70.0.3538.77 Safari/537.36",
		"--disable-gpu",
		"--disable-impl-side-painting",
		"--disable-gpu-sandbox",
		"--disable-accelerated-2d-canvas",
		"--disable-accelerated-jpeg-decoding",
		"--test-type=ui",
		"--ignore-certificate-errors",
	},
}

func main() {
	//1.开启selenium服务
	//设置selium服务的选项,设置为空。根据需要设置。

	ops := []selenium.ServiceOption{}
	service, err := selenium.NewChromeDriverService(seleniumPath, port, ops...)
	if err != nil {
		fmt.Printf("Error starting the ChromeDriver server: %v", err)
	}
	//延迟关闭服务
	defer service.Stop()

	//2.调用浏览器
	//设置浏览器兼容性，我们设置浏览器名称为chrome
	caps := selenium.Capabilities{
		"browserName": "chrome",
	}
	caps.AddChrome(chromeCaps)
	//调用浏览器urlPrefix: 测试参考：DefaultURLPrefix = "http://127.0.0.1:4444/wd/hub"
	wd, err := selenium.NewRemote(caps, "http://127.0.0.1:9515/wd/hub")
	if err != nil {
		panic(err)
	}
	//延迟退出chrome
	defer wd.Quit()

	// colly
	c := colly.NewCollector(
		colly.AllowURLRevisit(),
		//colly.CacheDir("./cache"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/92.0.4515.107 Safari/537.36"),
	)

	_ = c.Limit(&colly.LimitRule{
		Parallelism: 1,
		RandomDelay: 5 * time.Second,
	})

	products := new([]product)

	c.OnError(func(r *colly.Response, err error) {
		log.Println("Something went wrong:", err)
		time.Sleep(5 * time.Second)
		r.Request.Retry()
	})

	c.OnResponse(func(r *colly.Response) {
		log.Println("Visiting", r.Request.URL.String(), r.StatusCode)
	})

	// 列表页
	c.OnResponse(func(r *colly.Response) {
		doc, err := htmlquery.Parse(strings.NewReader(string(r.Body)))
		if err != nil {
			log.Fatal(err)
		}
		urls := htmlquery.Find(doc, `//div[@class='w-full md:w-1/2 xl:w-1/3 p-2']//a[contains(@class,"font-semibold")]/@href`)
		for _, url := range urls {
			if url == nil {
				continue
			}
			r.Request.Visit(htmlquery.InnerText(url))
		}
	})

	// 产品页
	c.OnHTML("div[x-data='AlpineComponentProduct()']", func(e *colly.HTMLElement) {
		p := product{}
		p.Title = e.ChildText("h1.text-3xl")
		p.Category = e.DOM.Find("a.flex-none").Eq(2).Text()
		p.Price = e.DOM.Find("div[class='font-bold text-3xl']").Text()

		// 简介
		shortTmp := e.DOM.Find("div[class='w-full lg:w-1/2 lg:ml-10'] > div.mt-5").Eq(2).Text()
		if strings.Contains(shortTmp, "Descriere detaliata") {
			p.ShortDescription = strings.TrimSpace(strings.Replace(shortTmp, "Descriere detaliata", "", -1))
		}

		// 描述
		longTmp, _ := e.DOM.Find("div[class='mt-5 static']").Html()
		longTmp = strings.Replace(strings.TrimSpace(longTmp), "\n", "", -1)
		longTmpMatch := regexp.MustCompile(`^(.+?)\s*<div`).FindStringSubmatch(longTmp)
		if longTmpMatch != nil {
			if !strings.Contains(longTmpMatch[1], "<div") {
				p.LongDescription = strings.ReplaceAll(longTmpMatch[1], "<p>", "")
				p.LongDescription = strings.ReplaceAll(p.LongDescription, "</p>", "")
			}
		}

		// 获取规格
		profileTmp, _ := e.DOM.Find("#attributes > div[class='mt-5 space-y-5']").Html()
		p.Profile = strings.Replace(strings.TrimSpace(profileTmp), "\n", "", -1)
		p.Profile = DeleteExtraSpace(p.Profile)

		// 图片
		var imgList []string
		e.ForEach("div[x-ref=gallery_top] > div.swiper-wrapper > div.swiper-slide", func(i int, el *colly.HTMLElement) {
			imgTmp := regexp.MustCompile(`image:url\((.+)_thumb(\.\w+)\)`).FindStringSubmatch(el.Attr("style"))
			if imgTmp == nil {
				return
			}
			imgList = append(imgList, fmt.Sprintf(imgTmp[1]+imgTmp[2]))
		})
		p.Images_color1 = strings.Join(imgList, ",")
		p.Url = e.Request.URL.String()

		// 颜色
		//colorTmp := e.DOM.Find("div[class='w-full lg:w-1/2 lg:ml-10'] > div.mt-3 > div.space-x-2 > button")
		//log.Printf(">>>>>> totally has %d colors", colorTmp.Length())

		//for i := 1; i < colorTmp.Length(); i++ {
		//retry:
		//	pics, status := fetJs(wd, e.Request.URL.String(), i)
		//
		//	if status == "" {
		//		time.Sleep(10 * time.Second)
		//		log.Printf(">>>>>> %d colors failed, retry", colorTmp.Length())
		//		goto retry
		//	} else if status == "drop" {
		//		continue
		//	}
		//
		//	switch i {
		//	case 1:
		//		p.Images_color2 = pics
		//	case 2:
		//		p.Images_color3 = pics
		//	case 3:
		//		p.Images_color4 = pics
		//	}
		//
		//}

		*products = append(*products, p)
	})

	// 页码
	for i := 1; i <= 12; i++ {
		if i == 1 {
			c.Visit("https://artflame.ro/category/kits")
		} else {
			c.Visit(fmt.Sprintf("https://artflame.ro/category/kits/%d", i))
		}
	}
	c.Wait()

	SaveCSV(products)

	log.Println("Job done!!!")
}

func SaveCSV(ps *[]product) {
	f, err := os.Create("result.csv") //创建文件
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString("\xEF\xBB\xBF")

	writer := csv.NewWriter(f)
	defer writer.Flush()

	//将爬取信息写入csv文件
	//writer.Write([]string{"title", "category", "price", "short description", "long description", "profile", "Images color1", "Images color2", "Images color3", "Images color4"})
	writer.Write([]string{"title", "category", "price", "short description", "long description", "profile", "Images color1"})
	for _, item := range *ps {
		//writer.Write([]string{item.Title, item.Category, item.Price, item.ShortDescription, item.LongDescription, item.Profile, item.Images_color1, item.Images_color2, item.Images_color3, item.Images_color4})
		writer.Write([]string{item.Title, item.Category, item.Price, item.ShortDescription, item.LongDescription, item.Profile, item.Images_color1})
	}
}

// 返回 ok 为成功，空为失败， drop 为放弃
func fetJs(wd selenium.WebDriver, url string, index int) (imgs string, status string) {
	//3.对页面元素进行操作
	//获取百度页面
	log.Printf("FetJs open %s for the %dth color", url, index+1)
	if err := wd.Get(url); err != nil {
		log.Println(err)
		return
	}
	//defer wd.Close()

	time.Sleep(8 * time.Second)

	//找到按钮
	we, err := wd.FindElements(selenium.ByCSSSelector, "div[class='w-full lg:w-1/2 lg:ml-10'] > .mt-3 > div[class='flex items-center space-x-2'] > button")
	if err != nil {
		log.Println(err)
		return
	}

	// 按下对应的按钮
	if len(we) > 1 {
		err := we[index].Click()
		if err != nil {
			log.Println(err)
			return
		}
	}

	time.Sleep(8 * time.Second)

	//获取网页源码
	we, err = wd.FindElements(selenium.ByCSSSelector, "div[x-ref=gallery_top] > div.swiper-wrapper > div.swiper-slide")
	if err != nil {
		log.Println(err)
		return
	}

	var imgList []string
	for _, item := range we {
		// 从 style 获取code
		imgTmp1, err := item.GetAttribute("style")
		if err != nil {
			log.Printf("fail when FetJs try to get %s image: %d\n", url, index)
			continue
		}

		//println(imgTmp1)
		//

		// 该产品没有图片
		if strings.Contains(imgTmp1, "image-not-found-placeholder.jpg") {
			return "", "drop"
		}

		// 提取图片
		imgTmp2 := regexp.MustCompile(`url\("(.+?)_thumb(\.\w+)"\);`).FindStringSubmatch(imgTmp1)
		if imgTmp2 == nil {
			continue
		}
		imgList = append(imgList, fmt.Sprintf(imgTmp2[1]+imgTmp2[2]))
		//println(fmt.Sprintf(imgTmp2[1] + imgTmp2[2]))
	}

	return strings.Join(imgList, ","), "ok"
}

func DeleteExtraSpace(s string) string {
	//删除字符串中的多余空格，有多个空格时，仅保留一个空格
	s1 := strings.Replace(s, "  ", " ", -1)      //替换tab为空格
	regstr := "\\s{2,}"                          //两个及两个以上空格的正则表达式
	reg, _ := regexp.Compile(regstr)             //编译正则表达式
	s2 := make([]byte, len(s1))                  //定义字符数组切片
	copy(s2, s1)                                 //将字符串复制到切片
	spc_index := reg.FindStringIndex(string(s2)) //在字符串中搜索
	for len(spc_index) > 0 {                     //找到适配项
		s2 = append(s2[:spc_index[0]+1], s2[spc_index[1]:]...) //删除多余空格
		spc_index = reg.FindStringIndex(string(s2))            //继续在字符串中搜索
	}
	return string(s2)
}
