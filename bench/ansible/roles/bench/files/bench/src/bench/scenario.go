package bench

import (
	"bench/counter"
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var (
	loginReg = regexp.MustCompile(`^/login$`)
)

func getCsrfToken(checker *Checker, ctx context.Context, url string) (csrf_token string, err error) {
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               url,
		ExpectedStatusCode: 200,
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if url == "/login" {
				csrf_token, _ = doc.Find("body > form > input").First().Attr("value")
			} else if url == "/bulletins/add" {
				csrf_token, _ = doc.Find("body > main > div.container > div.py-3 > form > input").First().Attr("value")
			} else {
				csrf_token, _ = doc.Find("body > div.container > div.py-3 > form > input").First().Attr("value")
			}
			return nil
		}),
		Description: "csrf_tokenを取得",
	})
	return csrf_token, err
}

func checkHTML(f func(*http.Response, *goquery.Document) error) func(*http.Response, *bytes.Buffer) error {
	return func(res *http.Response, body *bytes.Buffer) error {
		doc, err := goquery.NewDocumentFromReader(body)
		if err != nil {
			return fatalErrorf("ページのHTMLがパースできませんでした")
		}
		return f(res, doc)
	}
}

func genPostImageBody(fileName string, postBodyNames []string, postBodyValues map[string]string) (*bytes.Buffer, string, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	if len(postBodyNames) != len(postBodyValues) {
		// return nil, "", "POSTのデータに不備があります"
	}
	for i := 0; i < len(postBodyNames); i++ {
		writer.WriteField(postBodyNames[i], postBodyValues[postBodyNames[i]])
	}

	imageNum := rand.Perm(len(UploadFileImages) - 1)[0]
	image := UploadFileImages[imageNum]

	uploadFile := filepath.Join(DataPath, image.Path)
	fileWriter, err := writer.CreateFormFile("icon", fileName)
	if err != nil {
		return nil, "", err
	}

	readFile, err := os.Open(uploadFile)
	if err != nil {
		return nil, "", err
	}
	defer readFile.Close()

	io.Copy(fileWriter, readFile)
	writer.Close()

	return body, writer.FormDataContentType(), err
}

func checkRedirectStatusCode(res *http.Response, body *bytes.Buffer) error {
	if res.StatusCode == 302 || res.StatusCode == 303 {
		return nil
	}
	return fmt.Errorf("期待していないステータスコード %d Expected 302 or 303", res.StatusCode)
}

func checkRedirectStatusCodeError(res *http.Response, body *bytes.Buffer) error {
	if res.StatusCode == 403 {
		return nil
	}
	return fmt.Errorf("期待していないステータスコード %d Expected 403", res.StatusCode)
}

func PreAddUser(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	_, checker2, push2 := state.PopRandomUser()
	//_, _, push2 := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()
	defer push2()

	// 新規ユーザ追加
	url := "/users/add"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	fileName := RandomAlphabetString(20) + ".png"
	newUser := RandomAlphabetString(4)
	newPass := RandomAlphabetString(8)
	postBodyNames := []string{"username", "password", "password_confirm", "nickname", "csrf_token"}
	postBodyValues := make(map[string]string)
	postBodyValues["username"] = newUser
	postBodyValues["password"] = newPass
	postBodyValues["password_confirm"] = newPass
	postBodyValues["nickname"] = newUser + "-san"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err := genPostImageBody(fileName, postBodyNames, postBodyValues)

	rand.Seed(time.Now().UnixNano())
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/add",
		ContentType: ctype,
		PostBody:    body,
		CheckFunc:   checkRedirectStatusCode,
		Description: "新規ユーザ追加できること",
	})
	if err != nil {
		return err
	}

	// 新規登録したユーザでログイン
	url = "/login"
	csrf_token, err = getCsrfToken(checker2, ctx, url)

	if err != nil {
		return err
	}

	err = checker2.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       newUser,
			"password":   newPass,
			"csrf_token": csrf_token,
		},
		Description: "作成したユーザでログインできること",
	})
	if err != nil {
		return err
	}

	err = checker2.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	addUser := &AppUser{
		Name:     newUser,
		Password: newPass,
	}

	state.userMap[addUser.Name] = addUser
	state.users = append(state.users, addUser)

	return nil
}

// ログインユーザが投稿した記事、コメントは編集・削除ボタンが表示される
// ログインユーザ以外の記事、コメントに対しては編集・削除ボタンが表示されない
func CheckLayoutPreTest(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// takefusaユーザでログイン
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       "takefusa",
			"password":   "cXjCH2Cc",
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ログイン後トップページが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins",
		ExpectedStatusCode: 200,
		Description:        "トップページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > div.bulletin-add > a > button.bulletin-add-btn").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-title").Length() != 10 {
				return fatalErrorf("アクセスランキングが10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.pagination > ul > li").Length() < 10 {
				return fatalErrorf("ページネーションが正常に表示されていません")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	search_url := "/bulletins/search?title=AOPEN"
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "「AOPEN」が含まれるタイトルのみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			str := doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").First().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			str = doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").Last().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.pagination > ul > li").Length() < 5 {
				return fatalErrorf("ページネーションが正常に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// takefusaが投稿した社報が検索して表示されること
	search_url = "/bulletins/search?my_bulletins=takefusa"
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "takefusaが投稿した社報のみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.pagination-info > div.pagination-page-info > b").Last().Text() != "0" {
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").First().Text() != "takefusa" {
					return fatalErrorf("ニックネームがtakefusaではありません。")
				}
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").Last().Text() != "takefusa" {
					return fatalErrorf("ニックネームがtakefusaではありません。")
				}
			}
			if doc.Find("body > div.index-contents > div.row > div.pagination > ul > li").Length() < 5 {
				return fatalErrorf("ページネーションが正常に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 存在しない文字列でタイトル検索しても200が応答するかどうか
	search_url = "/bulletins/search?title=testtesttest"
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "ありえない文字列で検索しても200にならないこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 社報詳細ページのコメントが更新時間の昇順で表示されるか(29件あるはず)
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins/view/10",
		ExpectedStatusCode: 200,
		Description:        "新規社報ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			selection := doc.Find("body > div.container > div.container > div.comment-box > div.row > ul.mr-auto > li.list-inline-item > ul.list-unstyled > li.created")
			if selection.Text() == "" {
				return fatalErrorf("comment-boxが正常に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > div.container-fluid").Length() < 29 {
				return fatalErrorf("1/bulletins/view/10でコメントが29件表示されていません。")
			}
			format := "2006-01-02 15:04:05"
			tmp_mod, _ := time.Parse(format, "2000-01-01 15:04:05")
			flag_mod := 0
			selection.Each(func(index int, s *goquery.Selection) {
				str_mod := s.Text()
				t, _ := time.Parse(format, str_mod)
				// 時刻tがtmp_modより過去だったらNG。
				if t.Before(tmp_mod) {
					flag_mod += 1
				}
				tmp_mod = t
			})
			if flag_mod != 0 {
				return fatalErrorf("コメントが更新時間の昇順で表示されていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 社報投稿ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins/add",
		ExpectedStatusCode: 200,
		Description:        "社報投稿ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > input.add-title-input").Length() == 0 {
				return fatalErrorf("タイトル入力が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.add-description-input").Length() == 0 {
				return fatalErrorf("本文入力が適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// takefusaユーザで新規社報を追加できるか
	url = "/bulletins/add"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add",
		CheckFunc:   checkRedirectStatusCode,
		Description: "社報の新規投稿ができること",
		PostData: map[string]string{
			"title":      "benchmark",
			"body":       "benchmark",
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 追加した社報がトップページの上部に表示されるか
	viewURL := ""
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins",
		ExpectedStatusCode: 200,
		Description:        "新規社報がトップページに正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > div.bulletin-add > a > button.bulletin-add-btn").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-title").Length() != 10 {
				return fatalErrorf("アクセスランキングが10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.pagination > ul > li").Length() < 10 {
				return fatalErrorf("ページネーションが正常に表示されていません")
			}
			viewURL, _ = doc.Find("table.table-striped").Children().Find("td.table-contents-title > a").First().Attr("href")

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加した社報ページが正常に表示されるか(たぶん/bulletins/view/5001)
	view_editURL := ""
	star_count := ""
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "新規社報ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.row > h2.view-title").Text() != "benchmark" {
				return fatalErrorf("登録したタイトルが正常に表示されていません")
			}
			if !strings.Contains(doc.Find("body > div.container > div.bulletin-box > div.row > div.col").Text(), "benchmark") {
				return fatalErrorf("登録した本文が正常に表示されていません")
			}
			if doc.Find("body > div.container > div.bulletin-box > div.row > div.tttt > button > font").Text() != "編集" {
				return fatalErrorf("編集ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container-fluid > div.row > ul.list-inline > li.list-inline-item").Last().Text() != "0" {
				return fatalErrorf("新規投稿した社報本文のスター数が0ではありません。")
			}
			view_editURL, _ = doc.Find("button.bulletin-edit-btn").Attr("onclick")
			slice := strings.Split(view_editURL, "=")
			view_editURL = strings.Replace(slice[1], "'", "", -1)
			star_count = doc.Find("body > div.container > div.bulletin-box > div.row > ul > #star").Text()

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// スターを追加
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/star",
		ExpectedStatusCode: 200,
		Description:        "スターが追加できること",
		PostData: map[string]string{
			"bulletin_id": strings.Split(view_editURL, "/")[3],
			"csrf_token":  csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// スターが追加されたか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "スターが追加されたこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.bulletin-box > div.row > div.tttt > button > font").Text() != "編集" {
				return fatalErrorf("編集ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			count := doc.Find("body > div.container > div.bulletin-box > div.row > ul > #star").Text()
			after_count, _ := strconv.Atoi(count)
			before_count, _ := strconv.Atoi(star_count)

			if after_count <= before_count {
				return fatalErrorf("スターが正常に追加できていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加した社報の編集ページが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               view_editURL,
		ExpectedStatusCode: 200,
		Description:        "新規社報の編集ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			value, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-10 > input.edit-title-input").Attr("value")
			if value != "benchmark" {
				return fatalErrorf("タイトルが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.edit-description-input").Text() != "benchmark" {
				return fatalErrorf("本文が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.open-modal > a").Text() != "社報を削除する" {
				return fatalErrorf("社報を削除するリンクが表示されていません")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加した社報を編集できるか
	url = view_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        view_editURL,
		CheckFunc:   checkRedirectStatusCode,
		Description: "社報の編集ができること",
		PostData: map[string]string{
			"title":      "benchmark-update",
			"body":       "benchmark-update",
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 編集した社報ページが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "社報ページが更新されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.row > h2.view-title").Text() != "benchmark-update" {
				return fatalErrorf("登録したタイトルが正常に表示されていません")
			}
			if !strings.Contains(doc.Find("body > div.container > div.bulletin-box > div.row > div.col").Text(), "benchmark-update") {
				return fatalErrorf("登録した本文が正常に表示されていません")
			}
			if doc.Find("body > div.container > div.bulletin-box > div.row > div.tttt > button > font").Text() != "編集" {
				return fatalErrorf("編集ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加した社報にコメントを追加
	bulletin_id := strings.Split(view_editURL, "/")[3]
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add_comment",
		CheckFunc:   checkRedirectStatusCode,
		Description: "投稿へのコメントを追加1",
		PostData: map[string]string{
			"comment":     "benchmark",
			"csrf_token":  csrf_token,
			"bulletin_id": bulletin_id,
		},
	})
	if err != nil {
		return err
	}

	// 追加したコメントが正常に表示されるか
	comment_editURL := ""
	comment_id := ""
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "新規コメントが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment").Text() != "benchmark" {
				return fatalErrorf("コメントが正常に表示されていません")
			}
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > div.tttt > button.comment-edit-btn > font").Text() != "編集" {
				return fatalErrorf("コメント編集ボタンが正常に表示されていません")
			}
			comment_editURL, _ = doc.Find("button.comment-edit-btn").Attr("onclick")
			slice := strings.Split(comment_editURL, "=")
			comment_editURL = strings.Replace(slice[1], "'", "", -1)
			comment_id = strings.Split(comment_editURL, "/")[3]
			star_count = doc.Find(fmt.Sprintf("body > div.container > div.container > div.comment-box > div.row > ul > #comment-star-%s", comment_id)).Text()
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// コメントにスターを追加
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/star",
		ExpectedStatusCode: 200,
		Description:        "コメントにスターが追加できること",
		PostData: map[string]string{
			"comment_id": strings.Split(comment_editURL, "/")[3],
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// コメントにスターが追加されたか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "コメントにスターが追加されたこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			count := doc.Find(fmt.Sprintf("body > div.container > div.container > div.comment-box > div.row > ul > #comment-star-%s", comment_id)).Text()
			after_count, _ := strconv.Atoi(count)
			before_count, _ := strconv.Atoi(star_count)

			if after_count <= before_count {
				return fatalErrorf("コメントにスターが正常に追加できていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加したコメントの編集ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               comment_editURL,
		ExpectedStatusCode: 200,
		Description:        "新規コメントの編集ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.edit-description-input").Text() != "benchmark" {
				return fatalErrorf("コメント本文が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.open-modal > a").Text() != "コメントを削除する" {
				return fatalErrorf("コメントを削除するリンクが表示されていません")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// コメントの編集
	url = comment_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	comment_id = strings.Split(comment_editURL, "/")[3]
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        comment_editURL,
		CheckFunc:   checkRedirectStatusCode,
		Description: "コメントの編集",
		PostData: map[string]string{
			"body":       "benchmark-update",
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 編集したコメントが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "編集したコメントが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment").Text() != "benchmark-update" {
				return fatalErrorf("コメントが正常に表示されていません")
			}
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > div.tttt > button.comment-edit-btn > font").Text() != "編集" {
				return fatalErrorf("コメント編集ボタンが正常に表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if !strings.Contains(icon, "takefusa.png") {
				return fatalErrorf("ユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// パスワード更新ページが正常に表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/password",
		ExpectedStatusCode: 200,
		Description:        "パスワード更新ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "takefusa" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-8 > input.current-password-input").Length() == 0 {
				return fatalErrorf("現在のパスワード入力フォームが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-8 > input.new-password-input").Length() == 0 {
				return fatalErrorf("新しいパスワード入力フォームが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-8 > input.new-password-confirm-input").Length() == 0 {
				return fatalErrorf("新しいパスワード確認用入力フォームが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "更新" {
				return fatalErrorf("更新ボタンが表示されていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// takefusaユーザのパスワード更新ができることを確認("testtest")
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/users/password",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"password_current": "cXjCH2Cc",
			"password":         "testtest",
			"password_confirm": "testtest",
			"csrf_token":       csrf_token,
		},
		Description: "パスワード更新ができることを確認",
	})
	if err != nil {
		return err
	}

	// takefusaユーザがログアウトできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	// suzukiでログインできること
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       "suzuki",
			"password":   "VQpAYL3UPwkN",
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// takefusaが投稿した社報が表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "他人の投稿した記事が表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "suzuki" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.row > h2.view-title").Text() != "benchmark-update" {
				return fatalErrorf("登録したタイトルが正常に表示されていません")
			}
			if !strings.Contains(doc.Find("body > div.container > div.bulletin-box > div.row > div.col").Text(), "benchmark-update") {
				return fatalErrorf("登録した本文が正常に表示されていません")
			}
			if doc.Find("body > div.container > div.bulletin-box > div.row > div.tttt > button > font").Text() == "編集" {
				return fatalErrorf("他ユーザの社報ページに編集ボタンが表示されています。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > div.tttt > button.comment-edit-btn > font").Text() == "編集" {
				return fatalErrorf("他ユーザのコメント編集ボタンが表示されています。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// suzukiユーザでコメントの追加
	//url = "/bulletins/add_comment"
	url = "/users/edit"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add_comment",
		CheckFunc:   checkRedirectStatusCode,
		Description: "投稿へのコメントを追加2",
		PostData: map[string]string{
			"comment":     "suzuki-comment",
			"csrf_token":  csrf_token,
			"bulletin_id": bulletin_id,
		},
	})
	if err != nil {
		return err
	}

	// 追加したコメントが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "新規コメントが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment > font").Last().Text() != "suzuki-comment" {
				return fatalErrorf("コメントが正常に表示されていません")
			}
			if doc.Find("body > div.container > div.container > div.comment-box > div.row > div.tttt > button.comment-edit-btn > font").Text() != "編集" {
				return fatalErrorf("コメント編集ボタンが正常に表示されていません")
			}
			comment_editURL, _ = doc.Find("button.comment-edit-btn").Attr("onclick")
			slice := strings.Split(comment_editURL, "=")
			comment_editURL = strings.Replace(slice[1], "'", "", -1)

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 追加したコメントの編集ページへアクセス
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               comment_editURL,
		ExpectedStatusCode: 200,
		Description:        "新規コメントの編集ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != "suzuki" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.edit-description-input").Text() != "suzuki-comment" {
				return fatalErrorf("コメント本文が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.open-modal > a").Text() != "コメントを削除する" {
				return fatalErrorf("コメントを削除するリンクが表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// コメントの削除
	comment_id = strings.Split(comment_editURL, "/")[3]
	url = comment_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/comment/delete/" + comment_id,
		CheckFunc:   checkRedirectStatusCode,
		Description: "コメント削除",
		PostData: map[string]string{
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// コメントが正常に削除されたか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "コメントが削除されたこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {

			if doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment > font").Last().Text() == "suzuki-comment" {
				return fatalErrorf("コメントが正常に削除されていません")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// suzukiユーザでログアウト
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	// takefusaユーザでログイン(testtestでログインできるかも)
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       "takefusa",
			"password":   "testtest",
			"csrf_token": csrf_token,
		},
		Description: "takefusaユーザで変更したPWでログインできること",
	})
	if err != nil {
		return err
	}

	// 社報の削除
	url = "/bulletins/edit/" + bulletin_id
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/delete/" + bulletin_id,
		CheckFunc:   checkRedirectStatusCode,
		Description: "社報削除",
		PostData: map[string]string{
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 削除した社報ページが表示されないこと
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 404,
		Description:        "削除した社報ページが表示されないこと",
	})
	if err != nil {
		return err
	}

	// ログアウト
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

// バリデーションの初期状態が守られているか
func CheckValidation(ctx context.Context, state *State) error {
	user1, checker, push := state.PopRandomUser()
	if user1 == nil {
		return nil
	}
	defer push()

	// 既存ユーザ suzuki で確認していく
	username := "suzuki"
	nickname := "suzuki-san"
	password := "VQpAYL3UPwkN"

	// ユーザ登録で登録済みのユーザ名で登録しようとするとはじく
	url := "/users/add"
	csrf_token, err := getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	fileName := RandomAlphabetString(10) + ".png"
	postBodyNames := []string{"username", "password", "password_confirm", "nickname", "csrf_token"}
	postBodyValues := make(map[string]string)
	postBodyValues["username"] = username
	postBodyValues["password"] = "aaaaaaaa"
	postBodyValues["password_confirm"] = "aaaaaaaa"
	postBodyValues["nickname"] = "testest"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err := genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/add",
		ContentType:        ctype,
		PostBody:           body,
		ExpectedStatusCode: 409,
		Description:        "既存ユーザ名で新規登録できないことを確認",
	})
	if err != nil {
		return err
	}

	// ユーザ登録で登録済みのニックネームにしようとするとはじく
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	postBodyValues["username"] = "testman"
	postBodyValues["password"] = "aaaaaaaa"
	postBodyValues["password_confirm"] = "aaaaaaaa"
	postBodyValues["nickname"] = nickname
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err = genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/add",
		ContentType:        ctype,
		PostBody:           body,
		ExpectedStatusCode: 409,
		Description:        "既存ニックネームで新規登録できないことを確認",
	})
	if err != nil {
		return err
	}
	// ユーザ登録でパスワードが8文字未満だとはじく
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	postBodyValues["username"] = "testman"
	postBodyValues["password"] = "aaaaaaa"
	postBodyValues["password_confirm"] = "aaaaaaa"
	postBodyValues["nickname"] = "testest"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err = genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/add",
		ContentType:        ctype,
		PostBody:           body,
		ExpectedStatusCode: 409,
		Description:        "パスワードが8文字未満だと新規登録できないことを確認",
	})
	if err != nil {
		return err
	}

	// suzukiでログイン
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       username,
			"password":   password,
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ユーザ編集で既存ユーザ名(takefusa)に変更しようとするとapp側のバリデーションによって変更されていないことを確認
	url = "/users/edit"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/edit",
		CheckFunc:   checkRedirectStatusCode,
		Description: "既存ユーザ名(takefusa)に変更しようとする",
		PostData: map[string]string{
			"username":   "takefusa",
			"nickname":   "",
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "既存ユーザ名(takefusa)に変更されていないこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() == "takefusa" {
				return fatalErrorf("ユーザ名が既に存在しているユーザ名に変更できるようになってしまっています。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			edit_user := doc.Find("body > div.container > div.py-3 > form > div.form-group > label.col-md-4 > ul > li.edit-user-li > font.edit-user").Text()
			u := edit_user[14:]
			if u == "takefusa" {
				return fatalErrorf("ユーザ名が既に存在しているユーザ名に変更できるようになってしまっています。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ユーザ編集で既存ニックネーム(takefusa)に変更しようとするとapp側のバリデーションによって変更されていないことを確認
	url = "/users/edit"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/edit",
		CheckFunc:   checkRedirectStatusCode,
		Description: "既存ニックネーム(takefusa)に変更しようとする",
		PostData: map[string]string{
			"username":   "",
			"nickname":   "takefusa",
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "既存ニックネーム(takefusa)に変更されていないこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() == "takefusa" {
				return fatalErrorf("ユーザ名が既に存在しているユーザ名に変更できるようになってしまっています。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			edit_nickname := doc.Find("body > div.container > div.py-3 > form > div.form-group > label.col-md-4 > ul > li.edit-nickname-li > font.edit-nickname").Text()
			n := edit_nickname[20:]
			if n == "takefusa" {
				return fatalErrorf("ニックネームが既に存在しているニックネームに変更できるようになってしまっています。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// パスワード編集で「現在のパスワード(VQpAYL3UPwkN)」が一致していないとはじく
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/password",
		ExpectedStatusCode: 409,
		Description:        "現在のパスワードが一致していないと更新できないことを確認",
		PostData: map[string]string{
			"password_current": "zzzzzzzz",
			"password":         "aaaaaaaa",
			"password_confirm": "aaaaaaaa",
			"csrf_token":       csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// パスワード編集で「新しいパスワード」が8文字未満だとはじく
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/password",
		ExpectedStatusCode: 409,
		Description:        "新しいパスワードが8文字未満だと更新できないことを確認",
		PostData: map[string]string{
			"password_current": password,
			"password":         "aaaaaa7",
			"password_confirm": "aaaaaa7",
			"csrf_token":       csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// パスワード編集で「新しいパスワード」と「新しいパスワード確認」が一致していなければはじく
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/password",
		ExpectedStatusCode: 409,
		Description:        "現在のパスワードが一致していないと更新できないことを確認",
		PostData: map[string]string{
			"password_current": password,
			"password":         "aaaaaaaa",
			"password_confirm": "bbbbbbbb",
			"csrf_token":       csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// パスワード編集で全入力が正しくなければはじく
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/password",
		ExpectedStatusCode: 409,
		Description:        "現在のパスワードが一致していないと更新できないことを確認",
		PostData: map[string]string{
			"password_current": "zzzzzzzz",
			"password":         "aaaaaa7",
			"password_confirm": "bbbbbbbb",
			"csrf_token":       csrf_token,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// 社報は更新日の降順であることを確認
func CheckOrder(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// ログインできること
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// トップページで社報が更新日時順(降順) アクセスランキングがアクセス数順(降順)になっているか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins",
		ExpectedStatusCode: 200,
		Description:        "トップページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > div.bulletin-add > a > button.bulletin-add-btn").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-title").Length() != 10 {
				return fatalErrorf("アクセスランキングが10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.pagination > ul > li").Length() < 10 {
				return fatalErrorf("ページネーションが正常に表示されていません")
			}
			// 社報更新日時順
			selection := doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-modified")
			if selection.Text() == "" {
				return fatalErrorf("社報のテーブルが正常に表示されていません。")
			}
			format := "2006-01-02 15:04:05"
			tmp_mod, _ := time.Parse(format, "2020-01-02 15:04:05")
			flag_mod := 0
			selection.Each(func(index int, s *goquery.Selection) {
				str_mod := s.Text()
				t, _ := time.Parse(format, str_mod)
				// 時刻tは引数tmp_modより未来でだったらNG
				if t.After(tmp_mod) {
					flag_mod += 1
				}
				tmp_mod = t
			})
			if flag_mod != 0 {
				return fatalErrorf("社報が更新日時順(降順)で表示されていません。")
			}
			// アクセスランキング順
			selection2 := doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-count")
			if selection2.Text() == "" {
				return fatalErrorf("社報のテーブルが正常に表示されていません。")
			}
			tmp_n := 99999999999999999
			flag_n := 0
			selection2.Each(func(index int, s *goquery.Selection) {
				str_n := s.Text()
				n, _ := strconv.Atoi(str_n)
				// nがtmp_nより大きいとNG
				if n > tmp_n {
					flag_n += 1
				}
				tmp_n = n
			})
			if flag_n != 0 {
				return fatalErrorf("アクセスランキングがアクセス数順(降順)で表示されていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// searchページで「AOPEN」を検索した時にで社報が更新日時順(降順)になって表示されているか
	search_url := "/bulletins/search?title=AOPEN"
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "「AOPEN」が含まれるタイトルのみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			str := doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").First().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			str = doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").Last().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.pagination > ul > li").Length() < 5 {
				return fatalErrorf("ページネーションが正常に表示されていません。")
			}
			// 社報更新日時順
			selection := doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-modified")
			if selection.Text() == "" {
				return fatalErrorf("社報のテーブルが正常に表示されていません。")
			}
			format := "2006-01-02 15:04:05"
			tmp_mod, _ := time.Parse(format, "2020-01-02 15:04:05")
			flag_mod := 0
			selection.Each(func(index int, s *goquery.Selection) {
				str_mod := s.Text()
				t, _ := time.Parse(format, str_mod)
				// 時刻tは引数tmp_modより未来でだったらNG
				if t.After(tmp_mod) {
					flag_mod += 1
				}
				tmp_mod = t
			})
			if flag_mod != 0 {
				return fatalErrorf("社報が更新日時順(降順)で表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// searchページで自分が投稿した社報が更新日時順(降順)になって表示されているか
	search_url = "/bulletins/search?my_bulletins=" + user.Name
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "takefusaが投稿した社報のみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.pagination-info > div.pagination-page-info > b").Last().Text() != "0" {
				// 自分のニックネームが表示されているか
				nickname := user.Name + "-san"
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").First().Text() != nickname {
					return fatalErrorf("ニックネームが" + nickname + "ではありません。")
				}
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").Last().Text() != nickname {
					return fatalErrorf("ニックネームが" + nickname + "ではありません。")
				}
				// 社報更新日時順
				selection := doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-modified")
				if selection.Text() == "" {
					return fatalErrorf("社報のテーブルが正常に表示されていません。")
				}
				format := "2006-01-02 15:04:05"
				tmp_mod, _ := time.Parse(format, "2020-01-02 15:04:05")
				flag_mod := 0
				selection.Each(func(index int, s *goquery.Selection) {
					str_mod := s.Text()
					t, _ := time.Parse(format, str_mod)
					// 時刻tは引数tmp_modより未来でだったらNG
					if t.After(tmp_mod) {
						flag_mod += 1
					}
					tmp_mod = t
				})
				if flag_mod != 0 {
					return fatalErrorf("社報が更新日時順(降順)で表示されていません。")
				}
			}
			return nil
		}),
	})

	if err != nil {
		return err
	}

	return nil
}

func CheckNotLoggedInUser(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	checker.ResetCookie()

	// "/" へのアクセスは "/bulletins" へリダイレクトされること
	err := checker.Play(ctx, &CheckAction{
		Method:           "GET",
		Path:             "/",
		CheckFunc:        checkRedirectStatusCode,
		ExpectedLocation: regexp.MustCompile(`^/bulletins$`),
		Description:      "リダイレクトされること",
	})
	if err != nil {
		return err
	}

	// "/login" ページが表示されることを確認
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/login",
		ExpectedStatusCode: 200,
		Description:        "ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu04").Text() != "ユーザ登録" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu03").Text() != "ログイン" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > form > #inputusername").Size() != 1 {
				return fatalErrorf("入力フォームが適切に表示されていません")
			}
			if doc.Find("body > form > #inputPassword").Size() != 1 {
				return fatalErrorf("入力フォームが適切に表示されていません")
			}
			if doc.Find("body > form > button").Text() != "Sign in" {
				return fatalErrorf("ログインボタンが適切に表示されていません")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// "/users/edit"へのアクセスは403を返すこと
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/users/edit",
		CheckFunc:   checkRedirectStatusCodeError,
		Description: "403エラーになること",
	})
	if err != nil {
		return err
	}

	// "/bulletins/add"へのアクセスは"/login"にリダイレクトされること
	err = checker.Play(ctx, &CheckAction{
		Method:           "GET",
		Path:             "/bulletins/add",
		CheckFunc:        checkRedirectStatusCode,
		ExpectedLocation: loginReg,
		Description:      "ログインページにリダイレクトされること",
	})
	if err != nil {
		return err
	}

	// "/users/add" が表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/add",
		ExpectedStatusCode: 200,
		Description:        "ユーザ登録ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu04").Text() != "ユーザ登録" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu03").Text() != "ログイン" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #user-name > font").Text() != "ユーザ名" {
				return fatalErrorf("ユーザ名が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #password > font").Text() != "パスワード" {
				return fatalErrorf("パスワードが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #password-confirm > font").Text() != "パスワード確認用" {
				return fatalErrorf("パスワード確認用が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #nickname > font").Text() != "ニックネーム" {
				return fatalErrorf("ニックネームが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #icon > font").Text() != "アイコン" {
				return fatalErrorf("アイコンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "登録" {
				return fatalErrorf("登録ボタンが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	return nil
}

// - 存在するユーザでログインすることを確認
// - ログアウトできることを確認
// - 存在しないユーザではログインできないことを確認
func CheckLogin(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// 存在するユーザでログイン
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ログアウトできることを確認
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	// 存在しないユーザではログインできないことを確認
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/login",
		ExpectedStatusCode: 403,
		PostData: map[string]string{
			"name":       RandomAlphabetString(30),
			"password":   RandomAlphabetString(30),
			"csrf_token": csrf_token,
		},
		Description: "存在しないユーザでログインできないこと",
	})
	if err != nil {
		return fatalErrorf("存在しないユーザでログインできないこと")
	}

	return nil
}

func CheckAddUser(ctx context.Context, state *State) error {
	// 既に存在するユーザ情報をランダム取得
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// 上記既存ユーザ名で新規登録できないことを確認
	url := "/users/add"
	csrf_token, err := getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	fileName := RandomAlphabetString(10) + ".png"
	postBodyNames := []string{"username", "password", "password_confirm", "nickname", "csrf_token"}
	postBodyValues := make(map[string]string)
	postBodyValues["username"] = user.Name
	postBodyValues["password"] = user.Password
	postBodyValues["password_confirm"] = user.Password
	postBodyValues["nickname"] = user.Name + "-san"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err := genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/users/add",
		ContentType:        ctype,
		PostBody:           body,
		ExpectedStatusCode: 409,
		Description:        "既存ユーザ名で新規登録できないことを確認",
	})
	if err != nil {
		return err
	}

	// 登録するユーザ名とパスワードを生成(ランダム文字列)
	newUser := RandomAlphabetString(10)
	newPass := RandomAlphabetString(10)

	// 登録前のユーザとパスワードでログインできないことを確認
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/login",
		ExpectedStatusCode: 403,
		PostData: map[string]string{
			"name":       newUser,
			"password":   newPass,
			"csrf_token": csrf_token,
		},
		Description: "登録前のユーザとパスワードでログインできないこと",
	})
	if err != nil {
		return fatalErrorf("登録前のユーザとパスワードでログインできないこと")
	}

	// 新規ユーザ登録
	url = "/users/add"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	fileName = RandomAlphabetString(10) + ".png"
	postBodyNames = []string{"username", "password", "password_confirm", "nickname", "csrf_token"}
	postBodyValues = make(map[string]string)
	postBodyValues["username"] = newUser
	postBodyValues["password"] = newPass
	postBodyValues["password_confirm"] = newPass
	postBodyValues["nickname"] = newUser + "-san"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err = genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/add",
		ContentType: ctype,
		PostBody:    body,
		CheckFunc:   checkRedirectStatusCode,
		Description: "新規ユーザを登録できること",
	})
	if err != nil {
		return err
	}

	// 新規登録したユーザでログイン
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       newUser,
			"password":   newPass,
			"csrf_token": csrf_token,
		},
		Description: "新規登録したユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != newUser {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if !strings.Contains(icon, "png") {
				return fatalErrorf("ユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ログアウト
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckLayout(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// ユーザ作成画面が表示されること
	err := checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/add",
		ExpectedStatusCode: 200,
		Description:        "ユーザ作成画面が表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu04").Text() != "ユーザ登録" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu03").Text() != "ログイン" {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #user-name > font").Text() != "ユーザ名" {
				return fatalErrorf("ユーザ名が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #password > font").Text() != "パスワード" {
				return fatalErrorf("パスワードが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #password-confirm > font").Text() != "パスワード確認用" {
				return fatalErrorf("パスワード確認用が表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #nickname > font").Text() != "ニックネーム" {
				return fatalErrorf("ニックネームが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > #icon > font").Text() != "アイコン" {
				return fatalErrorf("アイコンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "登録" {
				return fatalErrorf("登録ボタンが表示されていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 存在するユーザでログイン
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ログイン後トップページが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins",
		ExpectedStatusCode: 200,
		Description:        "トップページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > div.bulletin-add > a > button.bulletin-add-btn").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-title").Length() != 10 {
				return fatalErrorf("アクセスランキングが10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.pagination > ul > li").Length() < 10 {
				return fatalErrorf("ページネーションが正常に表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ログイン後社報投稿ページが正常に表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins/add",
		ExpectedStatusCode: 200,
		Description:        "社報投稿ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > input.add-title-input").Length() == 0 {
				return fatalErrorf("タイトル入力が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.add-description-input").Length() == 0 {
				return fatalErrorf("本文入力が適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 社報詳細ページが正常に表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins/view/1",
		ExpectedStatusCode: 200,
		Description:        "社報詳細ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if !strings.Contains(icon, "png") {
				return fatalErrorf("ユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ログアウト
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckImage(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// 存在するユーザでログインできることを確認
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:    "POST",
		Path:      "/login",
		CheckFunc: checkRedirectStatusCode,
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
		Description: "存在するユーザでログインできること",
	})
	if err != nil {
		return err
	}

	// ログイン後ユーザ編集ページでアイコンが表示されていることを確認
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページでアイコンが表示されていること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if !strings.Contains(icon, "png") {
				return fatalErrorf("ユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ユーザアイコン(画像)がアップロードできることを確認
	url = "/users/edit"
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	fileName := RandomAlphabetString(20) + ".png"
	postBodyNames := []string{"username", "nickname", "csrf_token"}
	postBodyValues := make(map[string]string)
	postBodyValues["username"] = ""
	postBodyValues["nickname"] = ""
	postBodyValues["csrf_token"] = csrf_token
	body, ctype, err := genPostImageBody(fileName, postBodyNames, postBodyValues)

	err = checker.Play(ctx, &CheckAction{
		//DisableSlowChecking: true,
		Method:      "POST",
		Path:        "/users/edit",
		ContentType: ctype,
		PostBody:    body,
		CheckFunc:   checkRedirectStatusCode,
		Description: "正常にユーザアイコン(画像)がアップロードできること",
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページでアップロードした画像が表示されることを確認
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページでアップロードした画像が表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if icon != "/static/icons/"+fileName {
				return fatalErrorf("アップロードしたユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// アイコンを変更したあとアップロードされたアイコンのパス(/static/icons/<画像ファイル>)へアクセスしてが正常に応答するかどうか
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/static/icons/" + fileName,
		Description: "静的ファイルが取得できること",
		CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
			if res.StatusCode == http.StatusOK {
				counter.IncKey("staticfile-200")
			} else {
				return fmt.Errorf("期待していないステータスコード %d", res.StatusCode)
			}

			hasher := md5.New()
			_, err := io.Copy(hasher, body)
			if err != nil {
				return fatalErrorf("レスポンスボディの取得に失敗 %v", err)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	// ログインできることを確認
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

func CheckStaticFiles(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	for _, staticFile := range StaticFiles {
		sf := staticFile
		err := checker.Play(ctx, &CheckAction{
			Method:               "GET",
			Path:                 sf.Path,
			Description:          "静的ファイルが取得できること",
			EnableCache:          true,
			SkipIfCacheAvailable: true,
			CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
				if res.StatusCode == http.StatusOK {
					counter.IncKey("staticfile-200")
				} else if res.StatusCode == http.StatusNotModified {
					counter.IncKey("staticfile-304")
				} else {
					return fmt.Errorf("期待していないステータスコード %d", res.StatusCode)
				}

				hasher := md5.New()
				_, err := io.Copy(hasher, body)
				if err != nil {
					return fatalErrorf("レスポンスボディの取得に失敗 %v", err)
				}

				return nil
			},
		})
		if err != nil {
			return err
		}
	}

	imageNum := rand.Perm(len(StaticFileImages) - 1)[0]
	image := StaticFileImages[imageNum]
	err := checker.Play(ctx, &CheckAction{
		Method:               "GET",
		Path:                 image.Path,
		Description:          "静的ファイルが取得できること",
		EnableCache:          true,
		SkipIfCacheAvailable: true,
		CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
			if res.StatusCode == http.StatusOK {
				counter.IncKey("staticfile-200")
			} else if res.StatusCode == http.StatusNotModified {
				counter.IncKey("staticfile-304")
			} else {
				return fmt.Errorf("期待していないステータスコード %d", res.StatusCode)
			}

			hasher := md5.New()
			_, err := io.Copy(hasher, body)
			if err != nil {
				return fatalErrorf("レスポンスボディの取得に失敗 %v", err)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadPostOperation(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// ログインできること
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/login",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログインできること",
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 新規投稿しつつリダイレクト先のパス(redirect_path)を取得
	url = "/bulletins/add"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	title := RandomAlphabetString(16)
	body := RandomAlphabetString(256)
	redirect_url, err := checker.PlayReturnRedirect(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add",
		CheckFunc:   checkRedirectStatusCode,
		Description: "新規投稿",
		PostData: map[string]string{
			"title":      title,
			"body":       body,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	slice := strings.Split(redirect_url, "/")
	viewURL := strings.Join(slice[3:], "/")
	viewURL = "/" + viewURL

	// 新規投稿した社報が閲覧できること
	view_editURL := ""
	star_count := ""
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "新規投稿した社報が表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.row > h2.view-title").Text() != title {
				return fatalErrorf("登録したタイトルが正常に表示されていません")
			}
			if !strings.Contains(doc.Find("body > div.container > div.bulletin-box > div.row > div.col").Text(), body) {
				return fatalErrorf("登録した本文が正常に表示されていません")
			}
			if doc.Find("body > div.container > div.bulletin-box > div.row > div.tttt > button > font").Text() != "編集" {
				return fatalErrorf("編集ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container > form > div.form-group > button > font").Text() != "コメントを追加" {
				return fatalErrorf("コメントを追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.container-fluid > div.row > ul.list-inline > li.list-inline-item").Last().Text() != "0" {
				return fatalErrorf("新規投稿した社報本文のスター数が0ではありません。")
			}
			view_editURL, _ = doc.Find("button.bulletin-edit-btn").Attr("onclick")
			slice := strings.Split(view_editURL, "=")
			view_editURL = strings.Replace(slice[1], "'", "", -1)
			star_count = doc.Find("body > div.container > div.bulletin-box > div.row > ul > #star").Text()
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// スターを追加
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/star",
		ExpectedStatusCode: 200,
		Description:        "スターが追加できること",
		PostData: map[string]string{
			"bulletin_id": strings.Split(view_editURL, "/")[3],
			"csrf_token":  csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// スターが追加されたか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "スターが追加されたこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			count := doc.Find("body > div.container > div.bulletin-box > div.row > ul > #star").Text()
			after_count, _ := strconv.Atoi(count)
			before_count, _ := strconv.Atoi(star_count)

			if after_count <= before_count {
				return fatalErrorf("スターが正常に追加できていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 新規投稿した社報を編集できること
	url = view_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	newTitle := RandomAlphabetString(24)
	newBody := RandomAlphabetString(512)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        view_editURL,
		CheckFunc:   checkRedirectStatusCode,
		Description: "新規投稿を編集",
		PostData: map[string]string{
			"title":      newTitle,
			"body":       newBody,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 編集した新規投稿の社報が表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "編集した新規投稿の社報が表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > div.container > div.row > h2.view-title").Text() != newTitle {
				return fatalErrorf("登録したタイトルが正常に表示されていません")
			}
			if !strings.Contains(doc.Find("body > div.container > div.bulletin-box > div.row > div.col").Text(), newBody) {
				return fatalErrorf("登録した本文が正常に表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 新規投稿した社報にコメントできること
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	bulletin_id := strings.Split(view_editURL, "/")[3]
	comText1 := RandomAlphabetString(10)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add_comment",
		CheckFunc:   checkRedirectStatusCode,
		Description: "投稿へのコメントを追加1",
		PostData: map[string]string{
			"comment":     comText1,
			"csrf_token":  csrf_token,
			"bulletin_id": bulletin_id,
		},
	})
	if err != nil {
		return err
	}
	comText2 := RandomAlphabetString(20)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add_comment",
		CheckFunc:   checkRedirectStatusCode,
		Description: "投稿へのコメントを追加1",
		PostData: map[string]string{
			"comment":     comText2,
			"csrf_token":  csrf_token,
			"bulletin_id": bulletin_id,
		},
	})
	if err != nil {
		return err
	}
	comText3 := RandomAlphabetString(30)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/add_comment",
		CheckFunc:   checkRedirectStatusCode,
		Description: "投稿へのコメントを追加1",
		PostData: map[string]string{
			"comment":     comText3,
			"csrf_token":  csrf_token,
			"bulletin_id": bulletin_id,
		},
	})
	if err != nil {
		return err
	}

	// 新規投稿へのコメントが表示されること
	comment_editURL := ""
	comment_id := ""
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "新規投稿したコメントが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			comment := doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment").Text()
			if !strings.Contains(comment, comText1) {
				return fatalErrorf("コメント1 が本文が正常に表示されていません")
			}
			if !strings.Contains(comment, comText2) {
				return fatalErrorf("コメント2 が本文が正常に表示されていません")
			}
			if !strings.Contains(comment, comText3) {
				return fatalErrorf("コメント3 が本文が正常に表示されていません")
			}
			comment_editURL, _ = doc.Find("button.comment-edit-btn").First().Attr("onclick")
			slice := strings.Split(comment_editURL, "=")
			comment_editURL = strings.Replace(slice[1], "'", "", -1)
			comment_id = strings.Split(comment_editURL, "/")[3]
			star_count = doc.Find(fmt.Sprintf("body > div.container > div.container > div.comment-box > div.row > ul > #comment-star-%s", comment_id)).Text()
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// コメントにスターを追加
	err = checker.Play(ctx, &CheckAction{
		Method:             "POST",
		Path:               "/star",
		ExpectedStatusCode: 200,
		Description:        "コメントにスターが追加できること",
		PostData: map[string]string{
			"comment_id": strings.Split(comment_editURL, "/")[3],
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// コメントにスターが追加されたか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "コメントにスターが追加されたこと",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			count := doc.Find(fmt.Sprintf("body > div.container > div.container > div.comment-box > div.row > ul > #comment-star-%s", comment_id)).Text()
			after_count, _ := strconv.Atoi(count)
			before_count, _ := strconv.Atoi(star_count)

			if after_count <= before_count {
				return fatalErrorf("コメントにスターが正常に追加できていません。")
			}

			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 新規投稿へのコメントを編集できること
	url = comment_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	newComText := RandomAlphabetString(40)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        comment_editURL,
		CheckFunc:   checkRedirectStatusCode,
		Description: "コメントの編集",
		PostData: map[string]string{
			"body":       newComText,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 編集したコメントが表示されること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 200,
		Description:        "編集したコメントが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			comment := doc.Find("body > div.container > div.container > div.comment-box > div.row > #comment").Text()
			if !strings.Contains(comment, newComText) {
				return fatalErrorf("コメントが正常に表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// コメントが削除できること
	url = "/users/edit"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	comment_id = strings.Split(comment_editURL, "/")[3]
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/comment/delete/" + comment_id,
		CheckFunc:   checkRedirectStatusCode,
		Description: "コメント削除",
		PostData: map[string]string{
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 社報が削除できること
	url = "/bulletins/edit/" + bulletin_id
	//url = view_editURL
	csrf_token, err = getCsrfToken(checker, ctx, url)
	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/bulletins/delete/" + bulletin_id,
		CheckFunc:   checkRedirectStatusCode,
		Description: "社報削除",
		PostData: map[string]string{
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 削除した社報が表示されないこと
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               viewURL,
		ExpectedStatusCode: 404,
		Description:        "削除したので 404 になること",
	})
	if err != nil {
		return err
	}

	// ログアウトできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadUserOperation(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// 新規ユーザ追加
	url := "/users/add"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	fileName := RandomAlphabetString(20) + ".png"
	newUser := RandomAlphabetString(4)
	newPass := RandomAlphabetString(8)
	postBodyNames := []string{"username", "password", "password_confirm", "nickname", "csrf_token"}
	postBodyValues := make(map[string]string)
	postBodyValues["username"] = newUser
	postBodyValues["password"] = newPass
	postBodyValues["password_confirm"] = newPass
	postBodyValues["nickname"] = newUser + "-san"
	postBodyValues["csrf_token"] = csrf_token

	body, ctype, err := genPostImageBody(fileName, postBodyNames, postBodyValues)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/add",
		ContentType: ctype,
		PostBody:    body,
		CheckFunc:   checkRedirectStatusCode,
		Description: "新規ユーザ追加できること",
	})
	if err != nil {
		return err
	}

	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	// ログインできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/login",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログインできること",
		PostData: map[string]string{
			"name":       newUser,
			"password":   newPass,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページが表示できること
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページが表示できること",
	})
	if err != nil {
		return err
	}

	// パスワードを更新できること
	url = "/users/password"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}
	updatePass := RandomAlphabetString(16)
	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/users/password",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ユーザ情報が更新できること(パスワード)",
		PostData: map[string]string{
			"password_current": newPass,
			"password":         updatePass,
			"password_confirm": updatePass,
			"csrf_token":       csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// ログアウトできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	// 変更したパスワードでログインできること
	url = "/login"
	csrf_token, err = getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/login",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログインできること",
		PostData: map[string]string{
			"name":       newUser,
			"password":   updatePass,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// ログアウトできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}

func LoadReadOperation(ctx context.Context, state *State) error {
	user, checker, push := state.PopRandomUser()
	if user == nil {
		return nil
	}
	defer push()

	// ログインできること
	url := "/login"
	csrf_token, err := getCsrfToken(checker, ctx, url)

	if err != nil {
		return err
	}

	err = checker.Play(ctx, &CheckAction{
		Method:      "POST",
		Path:        "/login",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログインできること",
		PostData: map[string]string{
			"name":       user.Name,
			"password":   user.Password,
			"csrf_token": csrf_token,
		},
	})
	if err != nil {
		return err
	}

	// 社報一覧が正常に表示されること
	rand.Seed(time.Now().UnixNano())
	page := rand.Perm(200)[0]
	page += 1
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               fmt.Sprintf("/bulletins?page=%d", page),
		ExpectedStatusCode: 200,
		Description:        "社報一覧が正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > div.bulletin-add > a > button.bulletin-add-btn").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-8 > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.col-4 > table > tbody > tr.ranking-contents > td.ranking-title").Length() != 10 {
				return fatalErrorf("アクセスランキングが10件表示されていません。")
			}
			if doc.Find("body > div.container-fluid > div.row > div.pagination > ul > li").Length() < 10 {
				return fatalErrorf("ページネーションが正常に表示されていません")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	search_url := "/bulletins/search?title=AOPEN"
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "「AOPEN」が含まれるタイトルのみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > table > tbody > tr.table-contents > td.table-contents-title").Length() != 10 {
				return fatalErrorf("社報が10件表示されていません。")
			}
			str := doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").First().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			str = doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-title > a").Last().Text()
			if strings.Index(str, "AOPEN") == -1 {
				return fatalErrorf("社報のタイトルに「AOPEN」が含まれていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.pagination > ul > li").Length() < 5 {
				return fatalErrorf("ページネーションが正常に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 自分が投稿した社報が検索して表示されること(社報ない可能性もあるのでゆるくチェック)
	search_url = "/bulletins/search?my_bulletins=" + user.Name
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               search_url,
		ExpectedStatusCode: 200,
		Description:        "takefusaが投稿した社報のみ表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > div.index-contents > div.row > div.col > div.bulletin-add > a > button.bulletin-add-button").Text() != "社報を追加" {
				return fatalErrorf("追加ボタンが適切に表示されていません。")
			}

			if doc.Find("body > div.index-contents > div.row > div.col > div.pagination-info > div.pagination-page-info > b").Last().Text() != "0" {
				nickname := user.Name + "-san"
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").First().Text() != nickname {
					return fatalErrorf("ニックネームが" + nickname + "ではありません。")
				}
				if doc.Find("body > div.index-contents > div.row > div.col > table.bulletins-table > tbody > tr.table-contents > td.table-contents-nickname").Last().Text() != nickname {
					return fatalErrorf("ニックネームが" + nickname + "ではありません。")
				}
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// 社報投稿ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/bulletins/add",
		ExpectedStatusCode: 200,
		Description:        "社報投稿ページが正常に表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > input.add-title-input").Length() == 0 {
				return fatalErrorf("タイトル入力が適切に表示されていません。")
			}
			if doc.Find("body > main > div.container > div.py-3 > form > div.form-group > div.col-md-10 > textarea.add-description-input").Length() == 0 {
				return fatalErrorf("本文入力が適切に表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	// ユーザ編集ページが表示されるか
	err = checker.Play(ctx, &CheckAction{
		Method:             "GET",
		Path:               "/users/edit",
		ExpectedStatusCode: 200,
		Description:        "ユーザ編集ページが表示されること",
		CheckFunc: checkHTML(func(res *http.Response, doc *goquery.Document) error {
			if doc.Find("body > nav > ul > li.nav-item > a > font").Text() != "HISUBA" {
				return fatalErrorf("プロジェクト名'HISUBA'が適切に表示されていません。")
			}
			if doc.Find("body > nav > form.form-inline > button.bulletin-search-button").Text() != "タイトル検索" {
				return fatalErrorf("検索ボタンが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu02").Text() != "ログアウト" {
				return fatalErrorf("ログアウトが適切に表示されていません。")
			}
			if doc.Find("body > nav > ul > li > #menu01").Text() != user.Name {
				return fatalErrorf("ログインユーザ名が適切に表示されていません。")
			}
			icon, _ := doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > img").Attr("src")
			if !strings.Contains(icon, "png") {
				return fatalErrorf("ユーザアイコンが表示されていません")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-user-input").Length() == 0 {
				return fatalErrorf("ユーザ名入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-6 > ul > li > input.edit-nickname-input").Length() == 0 {
				return fatalErrorf("ニックネーム入力フォームが適切に表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > ul > li > input.edit-icon").Length() == 0 {
				return fatalErrorf("画像選択ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.col-md-1 > button").Text() != "保存" {
				return fatalErrorf("保存ボタンが表示されていません。")
			}
			if doc.Find("body > div.container > div.py-3 > form > div.form-group > div.link-password > a").Text() != "パスワードを更新する" {
				return fatalErrorf("パスワード更新のリンクが表示されていません。")
			}
			return nil
		}),
	})
	if err != nil {
		return err
	}

	for i := 0; i < 20; i++ {
		rand.Seed(time.Now().UnixNano())
		id := rand.Perm(2562)[0]
		id = id + 1
		err = checker.Play(ctx, &CheckAction{
			Method:             "GET",
			Path:               fmt.Sprintf("/bulletins/view/%d", id),
			ExpectedStatusCode: 200,
			Description:        "記事詳細画面が表示されること",
		})
		if err != nil {
			return err
		}
	}

	for i := 0; i < 5; i++ {
		rand.Seed(time.Now().UnixNano())
		page := rand.Perm(200)[0]
		page += 1
		err = checker.Play(ctx, &CheckAction{
			Method:             "GET",
			Path:               fmt.Sprintf("/bulletins?page=%d", page),
			ExpectedStatusCode: 200,
			Description:        "記事一覧が表示されること",
		})
		if err != nil {
			return err
		}

	}

	imageNum := rand.Perm(len(StaticFileImages) - 1)[0]
	image := StaticFileImages[imageNum]
	err = checker.Play(ctx, &CheckAction{
		Method:               "GET",
		Path:                 image.Path,
		Description:          "静的ファイルが取得できること",
		EnableCache:          true,
		SkipIfCacheAvailable: true,
		CheckFunc: func(res *http.Response, body *bytes.Buffer) error {
			if res.StatusCode == http.StatusOK {
				counter.IncKey("staticfile-200")
			} else if res.StatusCode == http.StatusNotModified {
				counter.IncKey("staticfile-304")
			} else {
				return fmt.Errorf("期待していないステータスコード %d", res.StatusCode)
			}

			hasher := md5.New()
			_, err := io.Copy(hasher, body)
			if err != nil {
				return fatalErrorf("レスポンスボディの取得に失敗 %v", err)
			}
			return nil
		},
	})
	if err != nil {
		return err
	}

	// ログアウトできること
	err = checker.Play(ctx, &CheckAction{
		Method:      "GET",
		Path:        "/logout",
		CheckFunc:   checkRedirectStatusCode,
		Description: "ログアウトできること",
	})
	if err != nil {
		return err
	}

	return nil
}
