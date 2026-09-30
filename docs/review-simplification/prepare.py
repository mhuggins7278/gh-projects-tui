from pathlib import Path
import json
import sys
repo = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(__file__).resolve().parents[2]
out = Path('/tmp/gh-projects-tui-simplification-review')
out.mkdir(parents=True, exist_ok=True)
bench = r'''package ui
import (
 "crypto/sha256"
 "fmt"
 "math/rand"
 "sort"
 "strings"
 "testing"
 "github.com/mhuggins7278/gh-projects-tui/internal/github"
)
func opportunityModel(n int, search bool) Model {
 m := NewModel(nil)
 m.screen = screenBoard
 m.width, m.height = 123, 63
 m.view = &github.View{Name: "Review fixture", Layout: github.BoardLayout, SortByFields: []github.SortField{{Field: github.Field{Name:"Title",DataType:"TITLE"},Direction:"ASC"}}}
 for i := 0; i < n; i++ {
  title := fmt.Sprintf("Mixed CASE task %05d",i)
  if i%20 == 0 { title = "Needle "+title }
  m.items = append(m.items,github.Item{ID:fmt.Sprintf("item-%d",i),Content:&github.Content{Kind:"Issue",Title:title,Number:i+1,Repository:"fixture/repo"}})
 }
 rand.New(rand.NewSource(17)).Shuffle(len(m.items),func(i,j int){m.items[i],m.items[j]=m.items[j],m.items[i]})
 if search { m.filter = "needle" }
 return m
}
func BenchmarkReviewOpportunityNavigation(b *testing.B) {
 for _, n := range []int{1000,10000} {
  for _, search := range []bool{false,true} {
   b.Run(fmt.Sprintf("cards-%d/search-%v",n,search),func(b *testing.B){
    m := opportunityModel(n,search)
    b.ReportAllocs();b.ResetTimer()
    for i:=0;i<b.N;i++ { m.moveBoardCard(1-2*(i%2)); _ = m.View().Content }
   })
  }
 }
}
func BenchmarkReviewOpportunityColdDetail(b *testing.B) {
 for _, n := range []int{20,100} {
  b.Run(fmt.Sprintf("comments-%d",n),func(b *testing.B){
   m:=benchmarkDetailModel(n)
   b.ReportAllocs();b.ResetTimer()
   for i:=0;i<b.N;i++ { _ = detailLines(*m.detail,80,false) }
  })
 }
}
func TestReviewOpportunityDigest(t *testing.T) {
 h:=sha256.New()
 m:=opportunityModel(100,true)
 for i:=0;i<5;i++ { m.moveBoardCard(1);fmt.Fprintln(h,m.boardFocusID,m.View().Content) }
 for _, typ := range []string{"TITLE","NUMBER","TEXT","SINGLE_SELECT","ITERATION","POSITION","UNKNOWN"} {
  field:=github.Field{ID:"f",Name:"Custom",DataType:typ,Options:[]github.FieldOption{{ID:"a",Name:"A"},{ID:"b",Name:"B"}}}
  if typ=="TITLE" {field.Name="Title"}
  for _, dir:=range []string{"ASC","DESC"} {
   view:=&github.View{SortByFields:[]github.SortField{{Field:field,Direction:dir},{Field:github.Field{Name:"Title",DataType:"TITLE"},Direction:"ASC"}}}
   var items []github.Item
   for i,v:=range []string{"","9","12","-1.5","nonnumeric","NaN","Hello","hello","ÉTÉ","a","b"} {
    item:=github.Item{ID:fmt.Sprint(i),FieldValues:[]github.FieldValue{{FieldID:"f",Value:v,OptionID:v,Available:i%4!=0}},Content:&github.Content{Title:fmt.Sprintf("Title %d",i%3)}}
    if i==0 {item.Content=nil};items=append(items,item)
   }
   for _,item:=range sortedItemsForView(view,items) {fmt.Fprint(h,item.ID,",")}
  }
 }
 for _,width:=range []int{17,80} {
  m:=benchmarkDetailModel(3)
  m.detail.Content.Body="# Heading\n\n- One\n- Two\n\n[link](https://example.com) and **bold**\n\n```go\nfmt.Println(\"hello\")\n```"
  for i:=range m.detail.Comments {m.detail.Comments[i].Body=[]string{"[link](https://example.com) and *italic*","| A | B |\n|---|---|\n| x | y |"," > quote\n\nSecond paragraph"}[i]}
  fmt.Fprintln(h,strings.Join(detailLines(*m.detail,width,false),"\n"))
 }
 t.Logf("output digest: %x",h.Sum(nil))
}
'''
(out/'review_test.go').write_text(bench)
def overlay(name,replacements):
 paths={str(repo/'internal/ui/opportunity_review_test.go'):str(out/'review_test.go')}
 for file,text in replacements.items():
  target=out/(name+'-'+file)
  target.write_text(text)
  paths[str(repo/'internal/ui'/file)]=str(target)
 (out/(name+'.json')).write_text(json.dumps({'Replace':paths}))
overlay('baseline',{})
board=(repo/'internal/ui/board.go').read_text()
quick=board.replace('items := sortedItemsForView(m.view, m.items)\n\tif strings.TrimSpace(m.filter) == "" {\n\t\treturn items\n\t}', 'items := m.items\n\tif strings.TrimSpace(m.filter) == "" {\n\t\treturn sortedItemsForView(m.view, items)\n\t}')
quick=quick.replace('\treturn filtered\n}', '\treturn sortedItemsForView(m.view, filtered)\n}',1)
a=quick.index('func (m *Model) moveBoardLane(');z=quick.index('func minInt(',a)
quick=quick[:a]+quick[a:z].replace('m.rememberBoardFocus()', 'm.boardFocusID = lanes[m.boardLane].Items[m.boardCard].ID')+quick[z:]
overlay('quick',{'board.go':quick})
detail=(repo/'internal/ui/detail.go').read_text()
start=detail.index('func detailLines(');end=detail.index('func formatCommentTime(',start)
chunk=detail[start:end]
chunk=chunk.replace('lines := make([]string, 0, len(detail.Fields)+12)', '''renderer, rendererErr := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width-4), glamour.WithPreservedNewLines())
 render := func(body string, ignoredWidth int) []string {
  if rendererErr == nil {
   rendered, err := renderer.Render(body)
   if err == nil {
    rendered = strings.Trim(rendered,"\\n")
    if rendered == "" { return []string{""} }
    return strings.Split(rendered,"\\n")
   }
  }
  return renderMarkdownFallback(body,width-4)
 }
 lines := make([]string, 0, len(detail.Fields)+12)''')
chunk=chunk.replace('renderMarkdown(', 'render(')
overlay('renderer',{'detail.go':detail[:start]+chunk+detail[end:]})
import re
model=(repo/'internal/ui/model.go').read_text()
mutation=(repo/'internal/ui/mutation.go').read_text()
picker=(repo/'internal/ui/picker_test.go').read_text()
def remove_decl(text,start_marker):
 start=text.index(start_marker)
 # Every removed declaration is at package level, with closing brace at column zero.
 end=text.index('\n}',start)+2
 return text[:start]+text[end:]
dead_board=board
for marker in ['type laneItemsRequest struct {','func parallelLaneItemsRequests(']:
 dead_board=remove_decl(dead_board,marker)
start=dead_board.index('\t\tif m.itemsLoading && len(m.itemsLoadingLanes) == 0 {')
end=dead_board.index('\n\t}',start)
dead_board=dead_board[:start]+'\t\tlanes[index].Loading = m.itemsLoading'+dead_board[end:]
for marker in ['func (m *Model) laneItemsCmd(', 'func (m Model) updateLaneItems(', 'func appendUniqueItems(', 'type laneItemsMsg struct {']:
 model=remove_decl(model,marker)
model=model.replace('\tcase laneItemsMsg:\n\t\treturn m.updateLaneItems(msg)\n','')
pattern=r'^\t(?:m\.)?(?:itemsLanePending|itemsLoadingLanes|itemsFailedLanes)\b.*\n'
model=re.sub(pattern,'',model,flags=re.M)
mutation=re.sub(pattern,'',mutation,flags=re.M)
picker=picker.replace('if len(batch) != 2 || model.itemsLanePending != 0 || model.itemsLoadingLanes != nil {','if len(batch) != 2 {').replace('t.Fatalf("saved filter used lane queries: commands=%d pending=%d", len(batch), model.itemsLanePending)','t.Fatalf("saved filter used lane queries: commands=%d", len(batch))')
overlay('dead',{'board.go':dead_board,'model.go':model,'mutation.go':mutation,'picker_test.go':picker})
print('dead loader production lines removed:',sum((repo/'internal/ui'/file).read_text().count('\n')-text.count('\n') for file,text in {'board.go':dead_board,'model.go':model,'mutation.go':mutation}.items()))
keys=quick
start=keys.index('func sortedItemsForView(')
end=keys.index('func sortFieldsSupported(',start)
keys=keys[:start]+'''func sortedItemsForView(view *github.View, items []github.Item) []github.Item {
 ordered := append([]github.Item(nil),items...)
 if view == nil || positionOnly(view) || !sortFieldsSupported(view) { return ordered }
 type sortRow struct { index int; values []itemSortValue }
 rows := make([]sortRow,len(items))
 values := make([]itemSortValue,len(items)*len(view.SortByFields))
 descending := make([]bool,len(view.SortByFields))
 for f,field := range view.SortByFields { descending[f] = strings.EqualFold(field.Direction,"DESC") }
 for i,item := range items {
  rows[i] = sortRow{index:i,values:values[i*len(view.SortByFields):(i+1)*len(view.SortByFields)]}
  for f,field := range view.SortByFields {
   value:=itemSortValueFor(field.Field,item)
   value.text=strings.ToLower(value.text)
   rows[i].values[f]=value
  }
 }
 sort.SliceStable(rows,func(left,right int) bool {
  for f := range view.SortByFields {
   comparison:=compareNormalizedSortValues(rows[left].values[f],rows[right].values[f],descending[f])
   if comparison != 0 { return comparison < 0 }
  }
  return false
 })
 for i,row := range rows { ordered[i] = items[row.index] }
 return ordered
}

'''+keys[end:]
start=keys.index('func compareSortField(')
end=keys.index('func itemSortValueFor(',start)
chunk=keys[start:end]
chunk=chunk.replace('\tif !leftValue.present', ''' leftValue.text = strings.ToLower(leftValue.text)
 rightValue.text = strings.ToLower(rightValue.text)
 return compareNormalizedSortValues(leftValue,rightValue,descending)
}

func compareNormalizedSortValues(leftValue,rightValue itemSortValue,descending bool) int {
 if !leftValue.present''',1)
chunk=chunk.replace('strings.Compare(strings.ToLower(leftValue.text), strings.ToLower(rightValue.text))','strings.Compare(leftValue.text, rightValue.text)')
keys=keys[:start]+chunk+keys[end:]
overlay('keys',{'board.go':keys})
tests=(out/'review_test.go').read_text().replace('"math/rand"','"math/rand"\n "reflect"')
for name,next_name,newname in [('sortedItemsForView','sortFieldsSupported','reviewReferenceSorted'),('compareSortField','itemSortValueFor','reviewReferenceCompare')]:
 start=board.index('func '+name+'(')
 end=board.index('func '+next_name+'(',start)
 tests+='\n'+board[start:end].replace(name,newname,1).replace('compareSortField(', 'reviewReferenceCompare(')
start=detail.index('func detailLines(');end=detail.index('func formatCommentTime(',start)
tests+='\n'+detail[start:end].replace('func detailLines(', 'func reviewReferenceDetailLines(',1)
tests+=r'''
func TestReviewOpportunitySortEquivalence(t *testing.T) {
 for _,typ:=range []string{"TITLE","NUMBER","TEXT","SINGLE_SELECT","ITERATION"} {
  field:=github.Field{ID:"f",Name:"Custom",DataType:typ,Options:[]github.FieldOption{{ID:"a",Name:"A"},{ID:"b",Name:"B"}},Iterations:[]github.Iteration{{ID:"a",Title:"First",StartDate:"2026-01-01"},{ID:"b",Title:"Second",StartDate:"2026-02-01"}}}
  if typ=="TITLE" {field.Name="Title"}
  for _,dir:=range []string{"ASC","DESC"} {
   for seed:=int64(0);seed<40;seed++ {
    rng:=rand.New(rand.NewSource(seed))
    view:=&github.View{SortByFields:[]github.SortField{{Field:field,Direction:dir},{Field:github.Field{Name:"Title",DataType:"TITLE"},Direction:"DESC"}}}
    var items []github.Item
    for i:=0;i<150;i++ {
     v:=[]string{"","9","12","-1.5","invalid","NaN","Hello","hello","ÉTÉ","a","b"}[rng.Intn(11)]
     item:=github.Item{ID:fmt.Sprint(i),Content:&github.Content{Title:fmt.Sprintf("Title %d",rng.Intn(12))},FieldValues:[]github.FieldValue{{FieldID:"f",Value:v,OptionID:v,IterationID:v,Available:rng.Intn(5)!=0}}}
     if rng.Intn(10)==0 {item.Content=nil}
     items=append(items,item)
    }
    got,want:=sortedItemsForView(view,items),reviewReferenceSorted(view,items)
    if !reflect.DeepEqual(got,want) {t.Fatalf("sort differs: type=%s direction=%s seed=%d",typ,dir,seed)}
   }
  }
 }
}
func TestReviewOpportunityMarkdownEquivalence(t *testing.T) {
 for _,width:=range []int{4,17,80,140} {
  m:=benchmarkDetailModel(5)
  bodies:=[]string{"plain text","# Header\n\n[one](https://example.com) [two][ref]\n\n[ref]: https://example.org","| A | B |\n|---|---|\n| x | y |","```go\nfmt.Println(\"Hello\")\n```"," > quote\n\n- one\n- two\n\n### Heading"}
  m.detail.Content.Body=bodies[1]
  for i:=range m.detail.Comments {m.detail.Comments[i].Body=bodies[i]}
  if !reflect.DeepEqual(detailLines(*m.detail,width,false),reviewReferenceDetailLines(*m.detail,width,false)) {t.Fatalf("detail differs at width %d",width)}
 }
}
'''
(out/'review_test.go').write_text(tests)
adapter=r'''package github
import (
 "context"
 "encoding/json"
 "fmt"
 "strings"
 "testing"
)
func TestReviewOpportunityMembershipRead(t *testing.T) {
 for _,lean:=range []bool{false,true} {
  calls:=0
  base:=reviewGraphQLFunc(func(_ context.Context,q string,_ map[string]interface{},r interface{})error{
   calls++
   if strings.Contains(q,"ItemFieldValues") { return json.Unmarshal([]byte(`{"node":{"fieldValues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`),r) }
   nodes:=make([]map[string]interface{},100)
   for i:=range nodes {
    nodes[i]=map[string]interface{}{"id":fmt.Sprintf("item-%d",i)}
    if !strings.Contains(q,"MutationProjectItems") {
     nodes[i]["content"]=map[string]interface{}{"__typename":"Issue","id":fmt.Sprintf("issue-%d",i),"number":i+1,"title":"A card","state":"OPEN"}
     nodes[i]["fieldValues"]=map[string]interface{}{"nodes":[]interface{}{},"pageInfo":map[string]interface{}{"hasNextPage":true,"endCursor":"more-fields"}}
    } else if strings.Contains(q,"fieldValue")||strings.Contains(q,"content") {t.Fatal("membership query included unused data")}
   }
   payload:=map[string]interface{}{"organization":map[string]interface{}{"projectV2":map[string]interface{}{"items":map[string]interface{}{"nodes":nodes,"pageInfo":map[string]interface{}{"hasNextPage":false}}}}}
   b,err:=json.Marshal(payload);if err!=nil{return err};return json.Unmarshal(b,r)
  })
  client:=newClient(base,&fakeREST{})
  owner:=Owner{Login:"fixture",Kind:OrganizationOwner}
  var page ItemsPage;var err error
  if lean {page,err=client.PageMutationItems(context.Background(),owner,1,"",nil)} else {page,err=client.PageItems(context.Background(),owner,1,"","")}
  if err!=nil {t.Fatal(err)}
  if len(page.Items)!=100 {t.Fatal("lost IDs")}
  for i,item:=range page.Items {if item.ID!=fmt.Sprintf("item-%d",i) {t.Fatal("order changed")}}
  want:=101;if lean {want=1};if calls!=want {t.Fatalf("requests=%d want=%d",calls,want)}
  t.Logf("membership lean=%v: %d requests for 100 IDs with paginated field values",lean,calls)
 }
}
func TestReviewOpportunityIssueStateRead(t *testing.T) {
 calls,commentPages:=0,0
 base:=reviewGraphQLFunc(func(_ context.Context,q string,_ map[string]interface{},r interface{})error{
  calls++
  if strings.Contains(q,"comments(first:") {
   commentPages++
   count:=100;if commentPages==3 {count=50}
   comments:=make([]map[string]interface{},count)
   for i:=range comments {comments[i]=map[string]interface{}{"id":fmt.Sprintf("comment-%d-%d",commentPages,i),"body":"Reply","createdAt":"2026-09-29T12:00:00Z","author":map[string]interface{}{"login":"fixture"}}}
   payload:=map[string]interface{}{"node":map[string]interface{}{"comments":map[string]interface{}{"nodes":comments,"pageInfo":map[string]interface{}{"hasNextPage":commentPages<3,"endCursor":fmt.Sprintf("page-%d",commentPages)}}}}
   b,err:=json.Marshal(payload);if err!=nil{return err};return json.Unmarshal(b,r)
  }
  return json.Unmarshal([]byte(`{"organization":{"projectV2":{"fields":{"nodes":[],"pageInfo":{"hasNextPage":false}}}},"node":{"id":"item","content":{"__typename":"Issue","id":"issue","title":"Fixture","body":"Body","state":"CLOSED"},"fieldValues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`),r)
 })
 detail,err:=newClient(base,&fakeREST{}).LoadItemDetail(context.Background(),Owner{Login:"fixture",Kind:OrganizationOwner},1,"item")
 if err!=nil {t.Fatal(err)}
 if detail.Content==nil||detail.Content.State!="CLOSED"||len(detail.Comments)!=250||calls!=4 {t.Fatalf("unexpected detail: calls=%d comments=%d",calls,len(detail.Comments))}
 t.Logf("current state read: %d requests including %d comment pages for 250 comments",calls,commentPages)
}
'''
(out/'adapter_test.go').write_text(adapter)
(out/'adapter.json').write_text(json.dumps({'Replace':{str(repo/'internal/github/opportunity_review_test.go'):str(out/'adapter_test.go')}}))
combined_board=keys
for marker in ['type laneItemsRequest struct {','func parallelLaneItemsRequests(']:
 combined_board=remove_decl(combined_board,marker)
start=combined_board.index('\t\tif m.itemsLoading && len(m.itemsLoadingLanes) == 0 {')
end=combined_board.index('\n\t}',start)
combined_board=combined_board[:start]+'\t\tlanes[index].Loading = m.itemsLoading'+combined_board[end:]
renderer_path=out/'renderer-detail.go'
overlay('combined',{'board.go':combined_board,'model.go':model,'mutation.go':mutation,'picker_test.go':picker,'detail.go':renderer_path.read_text()})
