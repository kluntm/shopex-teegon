package mq

import (
	"container/list"
	"sync"
)

type SafeList struct {
	ListItem list.List
	ListMuex sync.RWMutex
}

func (this *SafeList) PushBack(item interface{}) *list.Element {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	return this.ListItem.PushBack(item)
}

func (this *SafeList) PushFront(item interface{}) *list.Element {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	return this.ListItem.PushFront(item)
}

func (this *SafeList) MoveToBack(item *list.Element) {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	this.ListItem.MoveToBack(item)
}

func (this *SafeList) MoveToFront(item *list.Element) {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	this.ListItem.MoveToFront(item)
}

func (this *SafeList) Front() *list.Element {
	this.ListMuex.RLock()
	defer this.ListMuex.RUnlock()
	return this.ListItem.Front()
}

func (this *SafeList) Length() int {
	this.ListMuex.RLock()
	defer this.ListMuex.RUnlock()
	return this.ListItem.Len()
}

func (this *SafeList) Remove(item *list.Element) {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	this.ListItem.Remove(item)
}

func (this *SafeList) Clear() {
	this.ListMuex.Lock()
	defer this.ListMuex.Unlock()
	for e := this.ListItem.Front(); e != nil; {
		this.ListItem.Remove(e)
		e = this.ListItem.Front()
	}
}

//ForEach 用于遍历list
//fn: 返回值两个bool，第一个标识是否继续遍历
func (this *SafeList) ForEach(fn func(*list.Element) bool) {
	if fn == nil {
		return
	}
	this.ListMuex.RLock()
	defer this.ListMuex.RUnlock()
	var next *list.Element
	for e := this.ListItem.Front(); e != nil; {
		next = e.Next()
		bEnd := fn(e)

		if bEnd {
			break
		}
		e = next
	}
}
