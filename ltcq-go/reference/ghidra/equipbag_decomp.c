// ===== EquipBag::SetCapacity @ 0060e186 =====

/* DWARF original prototype: void SetCapacity(EquipBag * this, int32_t grid) */

void __thiscall EquipBag::SetCapacity(EquipBag *this,int32_t grid)

{
  int32_t grid_local;
  EquipBag *this_local;
  
  this->capacity_ = grid;
  return;
}



// ===== EquipBag::Spaces @ 00623278 =====

/* DWARF original prototype: int32_t Spaces(EquipBag * this) */

int32_t __thiscall EquipBag::Spaces(EquipBag *this)

{
  int iVar1;
  size_t sVar2;
  EquipBag *this_local;
  
  iVar1 = this->capacity_;
  sVar2 = BaseContainer::Count((BaseContainer *)this);
  return iVar1 - (int)sVar2;
}



// ===== EquipBag::EquipBag @ 006e148c =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void EquipBag(EquipBag * this, Actor * actor) */

void __thiscall EquipBag::EquipBag(EquipBag *this,Actor *actor)

{
  Actor *actor_local;
  EquipBag *this_local;
  
  ItemBag::ItemBag(&this->super_ItemBag,actor);
  (this->super_ItemBag).super_BaseContainer._vptr_BaseContainer =
       (_func_int32_t_varargs **)&PTR__EquipBag_007d2d70;
  *(undefined4 *)&(this->super_ItemBag).super_BaseContainer.field_0x24 = 1;
  return;
}



// ===== EquipBag::Init::lexical_block_0::__lambda0::operator() @ 006e14c8 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: bool operator()(__lambda0 * __closure) */

bool __thiscall EquipBag::Init::lexical_block_0::__lambda0::operator()(__lambda0 *this)

{
  bool bVar1;
  size_type sVar2;
  EquipBag **this_1;
  reference local_28;
  size_t i;
  
  i = 0;
  while( true ) {
    sVar2 = std::vector<bool,_std::allocator<bool>_>::size
                      (&(this->__this->super_ItemBag).buf_index_);
    if (sVar2 <= i) {
      return true;
    }
    local_28 = std::vector<bool,_std::allocator<bool>_>::operator[]
                         (&(this->__this->super_ItemBag).buf_index_,i);
    bVar1 = std::_Bit_reference::operator_bool(&local_28);
    if (!bVar1) break;
    i = i + 1;
  }
  return false;
}



// ===== EquipBag::Init @ 006e1544 =====

/* WARNING: Removing unreachable block (ram,0x006e1677) */
/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: bool Init(EquipBag * this, DataPacketReader * data) */

bool __thiscall EquipBag::Init(EquipBag *this,DataPacketReader *data)

{
  bool bVar1;
  DataPacketReader *pDVar2;
  ulong uVar3;
  size_t __n;
  char *__src;
  void *__dest;
  double dVar4;
  DataPacketReader *data_local;
  EquipBag *this_local;
  __lambda0 check_buf_ok;
  uint32_t max_size;
  uint32_t size;
  uint32_t curr_index;
  reference local_48;
  size_t item_size;
  ItemData *data_1;
  size_t i;
  size_t index_size;
  
  if ((this->super_ItemBag).actor_ == (Actor *)0x0) {
    bVar1 = false;
  }
  else {
    bVar1 = Actor::CheckInitStep((this->super_ItemBag).actor_,0);
    if (bVar1) {
      bVar1 = Actor::CheckInitStep((this->super_ItemBag).actor_,4);
      if (bVar1) {
        bVar1 = true;
      }
      else {
        curr_index = 0;
        size = 0;
        max_size = 0;
        pDVar2 = DataPacketReader::operator>><unsigned_int>(data,&curr_index);
        pDVar2 = DataPacketReader::operator>><unsigned_int>(pDVar2,&size);
        DataPacketReader::operator>><unsigned_int>(pDVar2,&max_size);
        if (max_size != 0) {
          uVar3 = std::string::size();
          if (uVar3 < max_size) {
            std::string::resize((ulong)&(this->super_ItemBag).db_buf_);
            index_size = 0;
            if (max_size != 0) {
              dVar4 = ceil((double)max_size / DAT_007d2d50);
              if (DAT_007d2d58 <= dVar4) {
                index_size = (long)(dVar4 - DAT_007d2d58) ^ 0x8000000000000000;
              }
              else {
                index_size = (size_t)dVar4;
              }
            }
            std::vector<bool,_std::allocator<bool>_>::resize
                      (&(this->super_ItemBag).buf_index_,index_size,false);
          }
          __n = DataPacketReader::getAvaliableLength(data);
          __src = DataPacketReader::getOffsetPtr(data);
          __dest = (void *)std::string::operator[]((ulong)&(this->super_ItemBag).db_buf_);
          memcpy(__dest,__src,__n);
          local_48 = std::vector<bool,_std::allocator<bool>_>::operator[]
                               (&(this->super_ItemBag).buf_index_,(ulong)(curr_index >> 4) / 0xc35);
          std::_Bit_reference::operator=(&local_48,true);
        }
        check_buf_ok.__this = this;
        bVar1 = Init::lexical_block_0::__lambda0::operator()(&check_buf_ok);
        if (bVar1) {
          data_1 = (ItemData *)std::string::operator[]((ulong)&(this->super_ItemBag).db_buf_);
          uVar3 = std::string::size();
          item_size = uVar3 / 0x90;
          for (i = 0; i < item_size; i = i + 1) {
            BaseContainer::AddItem((BaseContainer *)this,data_1 + i,false);
          }
          Actor::OnFinishInitStep((this->super_ItemBag).actor_,4);
          std::string::clear();
          std::vector<bool,_std::allocator<bool>_>::clear(&(this->super_ItemBag).buf_index_);
          UpdataCapacity(this);
        }
        bVar1 = true;
      }
    }
    else {
      OutputMsg(rmError,"init order error. on equipbat init.");
      bVar1 = false;
    }
  }
  return bVar1;
}



// ===== EquipBag::OnDecomposeEquip @ 006e1874 =====

/* DWARF original prototype: bool OnDecomposeEquip(EquipBag * this, int64_t uid) */

bool __thiscall EquipBag::OnDecomposeEquip(EquipBag *this,int64_t uid)

{
  int64_t uid_local;
  EquipBag *this_local;
  
  return false;
}



// ===== EquipBag::AppendItem @ 006e1888 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* WARNING: Struct "GameEngine": ignoring overlapping field "net_msg_route" */
/* DWARF original prototype: int32_t AppendItem(EquipBag * this, int32_t id, int32_t count, char *
   log, bool showTip) */

int32_t __thiscall
EquipBag::AppendItem(EquipBag *this,int32_t id,int32_t count,char *log,bool showTip)

{
  int iVar1;
  uint lvl;
  int32_t iVar2;
  int32_t actorid;
  int32_t accountid;
  size_t sVar3;
  GameServer *this_00;
  char *pcVar4;
  GameEngine *this_01;
  ItemConfig *pIVar5;
  char *name;
  bool showTip_local;
  char *log_local;
  int32_t count_local;
  int32_t id_local;
  EquipBag *this_local;
  char id_str [64];
  char count_str [64];
  char level_str [64];
  char real_get_str [64];
  ItemConfig *conf;
  int32_t level;
  int32_t i;
  
  i = 0;
  count_local = count;
  while ((0 < count_local &&
         (sVar3 = BaseContainer::Count((BaseContainer *)this), sVar3 < (uint)this->capacity_))) {
    BaseContainer::AddItem((BaseContainer *)this,id,1,showTip);
    count_local = count_local + -1;
    i = i + 1;
  }
  if ((AppendItem(int,int,char_const*,bool)::log_sender == '\0') &&
     (iVar1 = __cxa_guard_acquire(&AppendItem(int,int,char_const*,bool)::log_sender), iVar1 != 0)) {
    this_00 = ServerInstance();
    AppendItem::lexical_block_0::log_sender = GameServer::GetLogClient(this_00);
    __cxa_guard_release(&AppendItem(int,int,char_const*,bool)::log_sender);
  }
  lvl = Actor::GetLevel((this->super_ItemBag).actor_);
  sprintf(id_str,"%d",(ulong)(uint)id);
  sprintf(count_str,"%d",(ulong)(uint)count_local);
  sprintf(real_get_str,"%d",(ulong)(uint)i);
  sprintf(level_str,"%d",(ulong)lvl);
  pcVar4 = Actor::GetAccount((this->super_ItemBag).actor_);
  iVar2 = (*(((this->super_ItemBag).actor_)->super_Entity)._vptr_Entity[0x19])
                    ((this->super_ItemBag).actor_);
  LogSender::LogCounter
            (AppendItem::lexical_block_0::log_sender,iVar2,pcVar4,level_str,"give item",id_str,
             count_str,real_get_str,log,"equip_bag","","",1);
  if ((AppendItem(int,int,char_const*,bool)::dkmLog == '\0') &&
     (iVar1 = __cxa_guard_acquire(&AppendItem(int,int,char_const*,bool)::dkmLog), iVar1 != 0)) {
    this_01 = GetGameEngine();
    AppendItem::lexical_block_0::dkmLog = GameEngine::GetDKMlogMgr(this_01);
    __cxa_guard_release(&AppendItem(int,int,char_const*,bool)::dkmLog);
  }
  pIVar5 = ItemConfigPool::GetItemConfig(id);
  if ((0 < i) && (pIVar5 != (ItemConfig *)0x0)) {
    iVar2 = pIVar5->type;
    pcVar4 = Actor::GetPf((this->super_ItemBag).actor_);
    name = Entity::GetEntityName(&((this->super_ItemBag).actor_)->super_Entity);
    actorid = (*(((this->super_ItemBag).actor_)->super_Entity)._vptr_Entity[0x19])
                        ((this->super_ItemBag).actor_);
    accountid = Actor::GetAccountId((this->super_ItemBag).actor_);
    DKMLogMgr::LogAddItem
              (AppendItem::lexical_block_0::dkmLog,accountid,actorid,name,lvl,pcVar4,id,i,0,iVar2,
               log);
  }
  return i;
}



// ===== EquipBag::AppendItem @ 006e1bc6 =====

/* DWARF original prototype: int32_t AppendItem(EquipBag * this, ItemData * data, char * log, bool
   showTip) */

int32_t __thiscall EquipBag::AppendItem(EquipBag *this,ItemData *data,char *log,bool showTip)

{
  int32_t iVar1;
  size_t sVar2;
  bool showTip_local;
  char *log_local;
  ItemData *data_local;
  EquipBag *this_local;
  
  if (data == (ItemData *)0x0) {
    iVar1 = 0;
  }
  else {
    sVar2 = BaseContainer::Count((BaseContainer *)this);
    if (sVar2 < (uint)this->capacity_) {
      BaseContainer::AddItem((BaseContainer *)this,data,showTip);
      iVar1 = 0;
    }
    else {
      iVar1 = 1;
    }
  }
  return iVar1;
}



// ===== EquipBag::AppendItem @ 006e1c32 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* WARNING: Struct "GameEngine": ignoring overlapping field "net_msg_route" */
/* DWARF original prototype: int32_t AppendItem(EquipBag * this, Item * pItem, char * log, bool
   showTip) */

int32_t __thiscall EquipBag::AppendItem(EquipBag *this,Item *pItem,char *log,bool showTip)

{
  int iVar1;
  uint lvl;
  uint uVar2;
  int32_t iVar3;
  int32_t count;
  int32_t itemId;
  int32_t actorid;
  int32_t accountid;
  size_t sVar4;
  GameServer *this_00;
  char *pcVar5;
  GameEngine *this_01;
  char *name;
  bool showTip_local;
  char *log_local;
  Item *pItem_local;
  EquipBag *this_local;
  char id_str [64];
  char count_str [64];
  char real_get_str [64];
  char level_str [64];
  int32_t level;
  int32_t real_get;
  
  real_get = 0;
  if (pItem == (Item *)0x0) {
    real_get = 0;
  }
  else {
    if (pItem != (Item *)0x0) {
      sVar4 = BaseContainer::Count((BaseContainer *)this);
      if (sVar4 < (uint)this->capacity_) {
        BaseContainer::AddItem((BaseContainer *)this,pItem,true,showTip);
        real_get = 1;
      }
    }
    if (AppendItem(Item*,char_const*,bool)::log_sender == '\0') {
      iVar1 = __cxa_guard_acquire(&AppendItem(Item*,char_const*,bool)::log_sender);
      if (iVar1 != 0) {
        this_00 = ServerInstance();
        AppendItem::lexical_block_0::log_sender = GameServer::GetLogClient(this_00);
        __cxa_guard_release(&AppendItem(Item*,char_const*,bool)::log_sender);
      }
    }
    lvl = Actor::GetLevel((this->super_ItemBag).actor_);
    uVar2 = Item::GetId(pItem);
    sprintf(id_str,"%d",(ulong)uVar2);
    sprintf(count_str,"%d",1);
    sprintf(real_get_str,"%d",(ulong)(uint)real_get);
    sprintf(level_str,"%d",(ulong)lvl);
    pcVar5 = Actor::GetAccount((this->super_ItemBag).actor_);
    iVar3 = (*(((this->super_ItemBag).actor_)->super_Entity)._vptr_Entity[0x19])
                      ((this->super_ItemBag).actor_);
    LogSender::LogCounter
              (AppendItem::lexical_block_0::log_sender,iVar3,pcVar5,level_str,"give item",id_str,
               count_str,real_get_str,log,"equip_bag","","",1);
    if (AppendItem(Item*,char_const*,bool)::dkmLog == '\0') {
      iVar1 = __cxa_guard_acquire(&AppendItem(Item*,char_const*,bool)::dkmLog);
      if (iVar1 != 0) {
        this_01 = GetGameEngine();
        AppendItem::lexical_block_0::dkmLog = GameEngine::GetDKMlogMgr(this_01);
        __cxa_guard_release(&AppendItem(Item*,char_const*,bool)::dkmLog);
      }
    }
    iVar3 = Item::GetType(pItem);
    count = Item::GetCount(pItem);
    itemId = Item::GetId(pItem);
    pcVar5 = Actor::GetPf((this->super_ItemBag).actor_);
    name = Entity::GetEntityName(&((this->super_ItemBag).actor_)->super_Entity);
    actorid = (*(((this->super_ItemBag).actor_)->super_Entity)._vptr_Entity[0x19])
                        ((this->super_ItemBag).actor_);
    accountid = Actor::GetAccountId((this->super_ItemBag).actor_);
    DKMLogMgr::LogAddItem
              (AppendItem::lexical_block_0::dkmLog,accountid,actorid,name,lvl,pcVar5,itemId,count,0,
               iVar3,log);
  }
  return real_get;
}



// ===== EquipBag::IsCanSmelt @ 006e1f90 =====

/* DWARF original prototype: bool IsCanSmelt(EquipBag * this, int32_t type, int64_t uid) */

bool __thiscall EquipBag::IsCanSmelt(EquipBag *this,int32_t type,int64_t uid)

{
  bool bVar1;
  Item *this_00;
  int64_t uid_local;
  int32_t type_local;
  EquipBag *this_local;
  Item *item;
  
  this_00 = ItemBag::GetItem(&this->super_ItemBag,uid);
  if (this_00 == (Item *)0x0) {
    bVar1 = false;
  }
  else {
    bVar1 = Item::isEquip(this_00);
    if (bVar1) {
      bVar1 = true;
    }
    else {
      bVar1 = false;
    }
  }
  return bVar1;
}



// ===== EquipBag::OnEquipSmelt @ 006e1fea =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void OnEquipSmelt(EquipBag * this, int32_t type,
   vector<long_int,_std::allocator<long_int>_> * itemIdList) */

void __thiscall
EquipBag::OnEquipSmelt
          (EquipBag *this,int32_t type,vector<long_int,_std::allocator<long_int>_> *itemIdList)

{
  bool bVar1;
  int iVar2;
  int32_t iVar3;
  int32_t actorid;
  int32_t accountid;
  size_type sVar4;
  ulong uVar5;
  ItemConfig *pIVar6;
  size_type sVar7;
  mapped_type *pmVar8;
  mapped_type *pmVar9;
  pointer ppVar10;
  GameEngine *this_00;
  char *pfid;
  char *name;
  pointer ppVar11;
  vector<long_int,_std::allocator<long_int>_> *itemIdList_local;
  int32_t type_local;
  EquipBag *this_local;
  map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_> itemList;
  map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_> currency;
  _Rb_tree_iterator<std::pair<int_const,_int>_> itor;
  _Rb_tree_iterator<std::pair<int_const,_int>_> it_1;
  _Rb_tree_const_iterator<std::pair<int_const,_int>_> it;
  int32_t newEquipId;
  pair<int,_int> local_b8;
  int32_t local_ac;
  const_iterator local_a8 [2];
  iterator local_98 [2];
  iterator local_88 [2];
  SmeltEquipRateConfig *config;
  SmeltConfig *smeltConfig;
  int32_t itemType;
  int32_t id;
  Item *item;
  int64_t *itemId;
  int32_t count;
  size_t i_1;
  bool qualityAdd;
  int32_t curRate;
  int32_t i;
  int32_t smeltCount;
  int32_t goldNum;
  bool bSmelt;
  
  bSmelt = false;
  goldNum = 0;
  smeltCount = 0;
  std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::map(&itemList);
                    /* try { // try from 006e203b to 006e203f has its CatchHandler @ 006e25dc */
  std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::map(&currency);
  sVar4 = std::vector<long_int,_std::allocator<long_int>_>::size(itemIdList);
  count = (int32_t)sVar4;
  i = 0;
  do {
    if (count <= i) {
      if (goldNum != 0) {
        Actor::ChangeGold((this->super_ItemBag).actor_,goldNum,"equip smelt");
      }
      it_1._M_node = (_Base_ptr)
                     std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                     ::begin(&currency);
      while( true ) {
        local_98[0] = std::
                      map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::
                      end(&currency);
        bVar1 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator!=(&it_1,local_98);
        if (!bVar1) break;
        ppVar11 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&it_1);
        iVar3 = ppVar11->second;
        ppVar11 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&it_1);
        Actor::ChangeCurrency((this->super_ItemBag).actor_,ppVar11->first,iVar3,"equip smelt");
        std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator++(&it_1);
      }
      itor._M_node = (_Base_ptr)
                     std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                     ::begin(&itemList);
      while( true ) {
        local_88[0] = std::
                      map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::
                      end(&itemList);
        bVar1 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator!=(&itor,local_88);
        if (!bVar1) break;
        ppVar11 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&itor);
        iVar3 = ppVar11->second;
        ppVar11 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&itor);
        Actor::GiveItem((this->super_ItemBag).actor_,ppVar11->first,iVar3,"equip smelt",true);
        std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator++(&itor);
      }
      if (bSmelt != false) {
        ReEquipSmelt(this,1,goldNum,&itemList);
      }
      iVar3 = smeltCount;
      OnActorSmeltEvent(this,smeltCount);
      std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::~map
                (&currency,iVar3);
      std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::~map
                (&itemList,iVar3);
      return;
    }
    itemId = std::vector<long_int,_std::allocator<long_int>_>::operator[](itemIdList,(long)i);
                    /* try { // try from 006e2093 to 006e25a7 has its CatchHandler @ 006e25c8 */
    bVar1 = IsCanSmelt(this,type,*itemId);
    if ((bVar1) && (item = ItemBag::GetItem(&this->super_ItemBag,*itemId), item != (Item *)0x0)) {
      id = Item::GetId(item);
      itemType = Item::GetType(item);
      bVar1 = BaseContainer::DeleteItem((BaseContainer *)this,*itemId,1);
      if (bVar1) {
        smeltCount = smeltCount + 1;
        bSmelt = true;
        smeltConfig = ItemConfigPool::GetSmeltConfig(id);
        if (smeltConfig != (SmeltConfig *)0x0) {
          uVar5 = wrand(100);
          curRate = (int32_t)uVar5;
          qualityAdd = false;
          i_1 = 0;
          while (sVar7 = std::vector<SmeltEquipRateConfig,_std::allocator<SmeltEquipRateConfig>_>::
                         size(&smeltConfig->equipRateList), i_1 < sVar7) {
            config = std::vector<SmeltEquipRateConfig,_std::allocator<SmeltEquipRateConfig>_>::
                     operator[](&smeltConfig->equipRateList,i_1);
            if (curRate < config->rate) {
              newEquipId = id + config->qualityAdd * 100;
              pIVar6 = ItemConfigPool::GetItemConfig(newEquipId);
              if (pIVar6 != (ItemConfig *)0x0) {
                local_ac = 1;
                std::pair<int,_int>::pair<int&,_int,_void>(&local_b8,&newEquipId,&local_ac);
                std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::
                insert<std::pair<int,_int>,_void>(&itemList,&local_b8);
              }
              qualityAdd = true;
              break;
            }
            curRate = curRate - config->rate;
            i_1 = i_1 + 1;
          }
          if (qualityAdd != false) goto LAB_006e240f;
          goldNum = goldNum + smeltConfig->goldNum;
          if (smeltConfig->stoneNum != 0) {
            pmVar8 = std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                     ::operator[](&itemList,&smeltConfig->stoneId);
            pmVar9 = std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                     ::operator[](&itemList,&smeltConfig->stoneId);
            *pmVar8 = smeltConfig->stoneNum + *pmVar9;
          }
          it._M_node = (_Base_ptr)
                       std::
                       map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::
                       begin(&smeltConfig->currency);
          while( true ) {
            local_a8[0] = std::
                          map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                          ::end(&smeltConfig->currency);
            bVar1 = std::_Rb_tree_const_iterator<std::pair<int_const,_int>_>::operator!=
                              (&it,local_a8);
            if (!bVar1) break;
            ppVar10 = std::_Rb_tree_const_iterator<std::pair<int_const,_int>_>::operator->(&it);
            iVar2 = ppVar10->second;
            ppVar10 = std::_Rb_tree_const_iterator<std::pair<int_const,_int>_>::operator->(&it);
            pmVar8 = std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>
                     ::operator[](&currency,&ppVar10->first);
            *pmVar8 = *pmVar8 + iVar2;
            std::_Rb_tree_const_iterator<std::pair<int_const,_int>_>::operator++(&it);
          }
        }
        if ((OnEquipSmelt(int,std::vector<long,std::allocator<long>>&)::dkmLog == '\0') &&
           (iVar2 = __cxa_guard_acquire(&OnEquipSmelt(int,std::vector<long,std::allocator<long>>&)::
                                         dkmLog), iVar2 != 0)) {
          this_00 = GetGameEngine();
          OnEquipSmelt::lexical_block_0::lexical_block_0_0::lexical_block_0_0_0::dkmLog =
               GameEngine::GetDKMlogMgr(this_00);
          __cxa_guard_release(&OnEquipSmelt(int,std::vector<long,std::allocator<long>>&)::dkmLog);
        }
        pfid = Actor::GetPf((this->super_ItemBag).actor_);
        iVar3 = Actor::GetLevel((this->super_ItemBag).actor_);
        name = Entity::GetEntityName(&((this->super_ItemBag).actor_)->super_Entity);
        actorid = (*(((this->super_ItemBag).actor_)->super_Entity)._vptr_Entity[0x19])
                            ((this->super_ItemBag).actor_);
        accountid = Actor::GetAccountId((this->super_ItemBag).actor_);
        DKMLogMgr::LogCostItem
                  (OnEquipSmelt::lexical_block_0::lexical_block_0_0::lexical_block_0_0_0::dkmLog,
                   accountid,actorid,name,iVar3,pfid,id,1,0,itemType,"EquipSmelt");
      }
    }
LAB_006e240f:
    i = i + 1;
  } while( true );
}



// ===== EquipBag::OnExpandCapacity @ 006e260a =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void OnExpandCapacity(EquipBag * this, int32_t rows) */

void __thiscall EquipBag::OnExpandCapacity(EquipBag *this,int32_t rows)

{
  uint32_t uVar1;
  int iVar2;
  int32_t iVar3;
  int32_t iVar4;
  ActorBasicData *pAVar5;
  BagExpandConfig *pBVar6;
  int64_t iVar7;
  int32_t rows_local;
  EquipBag *this_local;
  BagUpdateCapacityProto msg;
  BagExpandConfig *config;
  int32_t basic_grid;
  int32_t temp;
  int32_t need_yb;
  int32_t expanded_rows;
  
  if (0 < rows) {
    pAVar5 = Actor::GetBasicData((this->super_ItemBag).actor_);
    uVar1 = pAVar5->bag_grid;
    iVar3 = BagConfigPool::GetBaseSize();
    iVar4 = BagConfigPool::GetRowSize();
    expanded_rows = (int)(uVar1 - iVar3) / iVar4;
    need_yb = 0;
    for (temp = rows; 0 < temp; temp = temp + -1) {
      expanded_rows = expanded_rows + 1;
      pBVar6 = BagConfigPool::GetExpandConfig(expanded_rows);
      if (pBVar6 == (BagExpandConfig *)0x0) {
        return;
      }
      need_yb = need_yb + pBVar6->cost;
    }
    iVar7 = Actor::GetYuanBao((this->super_ItemBag).actor_);
    if (need_yb <= iVar7) {
      Actor::ChangeYuanBao((this->super_ItemBag).actor_,-need_yb,"expand bag",false);
      iVar2 = this->capacity_;
      iVar3 = BagConfigPool::GetRowSize();
      this->capacity_ = iVar2 + iVar3 * rows;
      iVar3 = BagConfigPool::GetRowSize();
      iVar2 = uVar1 + iVar3 * rows;
      Actor::SetBagGrid((this->super_ItemBag).actor_,iVar2);
      Protocol::BagUpdateCapacityProto::BagUpdateCapacityProto(&msg);
      msg.grid = iVar2;
      Actor::SendData<Protocol::BagUpdateCapacityProto>((this->super_ItemBag).actor_,&msg);
    }
  }
  return;
}



// ===== EquipBag::OnVipLevelUpdate @ 006e274c =====

/* DWARF original prototype: void OnVipLevelUpdate(EquipBag * this) */

void __thiscall EquipBag::OnVipLevelUpdate(EquipBag *this)

{
  EquipBag *this_local;
  
  UpdataCapacity(this);
  return;
}



// ===== EquipBag::ReEquipSmelt @ 006e2766 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void ReEquipSmelt(EquipBag * this, int32_t result, int32_t goldNum,
   map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_> * itemList) */

void __thiscall
EquipBag::ReEquipSmelt
          (EquipBag *this,int32_t result,int32_t goldNum,
          map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_> *itemList)

{
  bool bVar1;
  int iVar2;
  size_type sVar3;
  pointer ppVar4;
  ItemConfig *pIVar5;
  map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_> *itemList_local;
  int32_t goldNum_local;
  int32_t result_local;
  EquipBag *this_local;
  _Rb_tree_iterator<std::pair<int_const,_int>_> itor;
  ActorPacket AP;
  iterator local_18;
  DataPacket *sp;
  
  if (ReEquipSmelt(int,int,std::map<int,int,std::less<int>,std::allocator<std::pair<int_const,int>>>&)
      ::msg == '\0') {
    iVar2 = __cxa_guard_acquire(&ReEquipSmelt(int,int,std::map<int,int,std::less<int>,std::allocator<std::pair<int_const,int>>>&)
                                 ::msg);
    if (iVar2 != 0) {
      Protocol::EquipSmeltProto::EquipSmeltProto(&ReEquipSmelt::lexical_block_0::msg);
      __cxa_guard_release(&ReEquipSmelt(int,int,std::map<int,int,std::less<int>,std::allocator<std::pair<int_const,int>>>&)
                           ::msg);
    }
  }
  ReEquipSmelt::lexical_block_0::msg.result = result;
  ReEquipSmelt::lexical_block_0::msg.goldNum = goldNum;
  sVar3 = std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::size
                    (itemList);
  ReEquipSmelt::lexical_block_0::msg.itemCount = (int32_t)sVar3;
  sp = Actor::AllocPacket((this->super_ItemBag).actor_,&AP);
  DataPacket::writeBuf(sp,&ReEquipSmelt::lexical_block_0::msg,0xe);
  itor._M_node = (_Base_ptr)
                 std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::
                 begin(itemList);
  while( true ) {
    local_18 = std::map<int,_int,_std::less<int>,_std::allocator<std::pair<int_const,_int>_>_>::end
                         (itemList);
    bVar1 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator!=(&itor,&local_18);
    if (!bVar1) break;
    ppVar4 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&itor);
    pIVar5 = ItemConfigPool::GetItemConfig(ppVar4->first);
    if (pIVar5 != (ItemConfig *)0x0) {
      ppVar4 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&itor);
      DataPacket::operator<<_<int>(sp,&ppVar4->first);
      ppVar4 = std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator->(&itor);
      DataPacket::operator<<_<int>(sp,&ppVar4->second);
    }
    std::_Rb_tree_iterator<std::pair<int_const,_int>_>::operator++(&itor);
  }
  ActorPacket::flush(&AP);
  return;
}



// ===== EquipBag::OnActorSmeltEvent @ 006e28b8 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void OnActorSmeltEvent(EquipBag * this, int32_t smeltCount) */

void __thiscall EquipBag::OnActorSmeltEvent(EquipBag *this,int32_t smeltCount)

{
  ScriptValueList *params_00;
  int32_t smeltCount_local;
  EquipBag *this_local;
  ScriptValueList params;
  
  ScriptValueList::ScriptValueList(&params);
  ScriptValueList::operator<<_<Actor*>(&params,(this->super_ItemBag).actor_);
  ScriptValueList::operator<<_<tagActorEventID>(&params,aeSmeltEquip);
  ScriptValueList::operator<<_<int>(&params,smeltCount);
  params_00 = &params;
                    /* try { // try from 006e2945 to 006e2949 has its CatchHandler @ 006e295b */
  Actor::OnEvent((this->super_ItemBag).actor_,params_00,(ScriptValueList *)0x0);
  ScriptValueList::~ScriptValueList(&params,(int)params_00);
  return;
}



// ===== EquipBag::UpdataCapacity @ 006e2982 =====

/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void UpdataCapacity(EquipBag * this) */

void __thiscall EquipBag::UpdataCapacity(EquipBag *this)

{
  uint32_t uVar1;
  int32_t iVar2;
  ActorBasicData *pAVar3;
  EquipBag *this_local;
  
  pAVar3 = Actor::GetBasicData((this->super_ItemBag).actor_);
  uVar1 = pAVar3->bag_grid;
  iVar2 = Actor::GetVipLevel((this->super_ItemBag).actor_);
  iVar2 = BagConfigPool::GetVipGrid(iVar2);
  this->capacity_ = iVar2 + uVar1;
  pAVar3 = Actor::GetBasicData((this->super_ItemBag).actor_);
  if (pAVar3->monthcard == 1) {
    this->capacity_ = this->capacity_ + 100;
  }
  pAVar3 = Actor::GetBasicData((this->super_ItemBag).actor_);
  if (pAVar3->privilege_monthcard == 1) {
    this->capacity_ = this->capacity_ + 100;
  }
  return;
}



// ===== EquipBag::~EquipBag @ 006e2a52 =====

/* WARNING: Removing unreachable block (ram,0x006e2a7e) */
/* WARNING: Struct "ItemBag": ignoring overlapping field "bag_type_" */
/* DWARF original prototype: void ~EquipBag(EquipBag * this, int __in_chrg) */

void __thiscall EquipBag::~EquipBag(EquipBag *this,int __in_chrg)

{
  EquipBag *this_local;
  
  (this->super_ItemBag).super_BaseContainer._vptr_BaseContainer =
       (_func_int32_t_varargs **)&PTR__EquipBag_007d2d70;
  ItemBag::~ItemBag(&this->super_ItemBag,__in_chrg);
  return;
}



// ===== EquipBag::~EquipBag @ 006e2a8c =====

/* DWARF original prototype: void ~EquipBag(EquipBag * this, int __in_chrg) */

void __thiscall EquipBag::~EquipBag(EquipBag *this,int __in_chrg)

{
  EquipBag *this_local;
  
  ~EquipBag(this,__in_chrg);
  operator_delete(this);
  return;
}



// ===== ItemBag::ItemBag @ 006e310c =====

/* DWARF original prototype: void ItemBag(ItemBag * this, Actor * actor) */

void __thiscall ItemBag::ItemBag(ItemBag *this,Actor *actor)

{
  Actor *actor_local;
  ItemBag *this_local;
  
  BaseContainer::BaseContainer(&this->super_BaseContainer);
  (this->super_BaseContainer)._vptr_BaseContainer = (_func_int32_t_varargs **)&PTR__ItemBag_007d2f50
  ;
  *(undefined4 *)&(this->super_BaseContainer).field_0x24 = 0;
  this->actor_ = actor;
                    /* try { // try from 006e3156 to 006e315a has its CatchHandler @ 006e3182 */
  std::string::string(&this->db_buf_);
                    /* try { // try from 006e3166 to 006e316a has its CatchHandler @ 006e316d */
  std::vector<bool,_std::allocator<bool>_>::vector(&this->buf_index_);
  return;
}



// ===== ItemBag::~ItemBag @ 006e31a4 =====

/* WARNING: Removing unreachable block (ram,0x006e31f0) */
/* DWARF original prototype: void ~ItemBag(ItemBag * this, int __in_chrg) */

void __thiscall ItemBag::~ItemBag(ItemBag *this,int __in_chrg)

{
  ItemBag *this_local;
  
  (this->super_BaseContainer)._vptr_BaseContainer = (_func_int32_t_varargs **)&PTR__ItemBag_007d2f50
  ;
  std::vector<bool,_std::allocator<bool>_>::~vector(&this->buf_index_,__in_chrg);
  std::string::~string(&this->db_buf_);
  BaseContainer::~BaseContainer(&this->super_BaseContainer,__in_chrg);
  return;
}



// ===== ItemBag::~ItemBag @ 006e31fe =====

/* DWARF original prototype: void ~ItemBag(ItemBag * this, int __in_chrg) */

void __thiscall ItemBag::~ItemBag(ItemBag *this,int __in_chrg)

{
  ItemBag *this_local;
  
  ~ItemBag(this,__in_chrg);
  operator_delete(this);
  return;
}



// ===== ItemBag::Init::lexical_block_0::__lambda0::operator() @ 006e3224 =====

/* DWARF original prototype: bool operator()(__lambda0 * __closure) */

bool __thiscall ItemBag::Init::lexical_block_0::__lambda0::operator()(__lambda0 *this)

{
  bool bVar1;
  size_type sVar2;
  ItemBag **this_1;
  reference local_28;
  size_t i;
  
  i = 0;
  while( true ) {
    sVar2 = std::vector<bool,_std::allocator<bool>_>::size(&this->__this->buf_index_);
    if (sVar2 <= i) {
      return true;
    }
    local_28 = std::vector<bool,_std::allocator<bool>_>::operator[](&this->__this->buf_index_,i);
    bVar1 = std::_Bit_reference::operator_bool(&local_28);
    if (!bVar1) break;
    i = i + 1;
  }
  return false;
}



// ===== ItemBag::Init @ 006e32a0 =====

/* WARNING: Removing unreachable block (ram,0x006e33d3) */
/* DWARF original prototype: bool Init(ItemBag * this, DataPacketReader * data) */

bool __thiscall ItemBag::Init(ItemBag *this,DataPacketReader *data)

{
  bool bVar1;
  DataPacketReader *pDVar2;
  ulong uVar3;
  size_t __n;
  char *__src;
  void *__dest;
  double dVar4;
  DataPacketReader *data_local;
  ItemBag *this_local;
  __lambda0 check_buf_ok;
  uint32_t max_size;
  uint32_t size;
  uint32_t curr_index;
  reference local_48;
  size_t item_size;
  ItemData *data_1;
  size_t i;
  size_t index_size;
  
  if (this->actor_ == (Actor *)0x0) {
    bVar1 = false;
  }
  else {
    bVar1 = Actor::CheckInitStep(this->actor_,0);
    if (bVar1) {
      bVar1 = Actor::CheckInitStep(this->actor_,3);
      if (bVar1) {
        bVar1 = true;
      }
      else {
        curr_index = 0;
        size = 0;
        max_size = 0;
        pDVar2 = DataPacketReader::operator>><unsigned_int>(data,&curr_index);
        pDVar2 = DataPacketReader::operator>><unsigned_int>(pDVar2,&size);
        DataPacketReader::operator>><unsigned_int>(pDVar2,&max_size);
        if (max_size != 0) {
          uVar3 = std::string::size();
          if (uVar3 < max_size) {
            std::string::resize((ulong)&this->db_buf_);
            index_size = 0;
            if (max_size != 0) {
              dVar4 = ceil((double)max_size / DAT_007d2f18);
              if (DAT_007d2f20 <= dVar4) {
                index_size = (long)(dVar4 - DAT_007d2f20) ^ 0x8000000000000000;
              }
              else {
                index_size = (size_t)dVar4;
              }
            }
            std::vector<bool,_std::allocator<bool>_>::resize(&this->buf_index_,index_size,false);
          }
          __n = DataPacketReader::getAvaliableLength(data);
          __src = DataPacketReader::getOffsetPtr(data);
          __dest = (void *)std::string::operator[]((ulong)&this->db_buf_);
          memcpy(__dest,__src,__n);
          local_48 = std::vector<bool,_std::allocator<bool>_>::operator[]
                               (&this->buf_index_,(ulong)(curr_index >> 4) / 0xc35);
          std::_Bit_reference::operator=(&local_48,true);
        }
        check_buf_ok.__this = this;
        bVar1 = Init::lexical_block_0::__lambda0::operator()(&check_buf_ok);
        if (bVar1) {
          data_1 = (ItemData *)std::string::operator[]((ulong)&this->db_buf_);
          uVar3 = std::string::size();
          item_size = uVar3 / 0x90;
          for (i = 0; i < item_size; i = i + 1) {
            BaseContainer::AddItem(&this->super_BaseContainer,data_1 + i,false);
          }
          Actor::OnFinishInitStep(this->actor_,3);
          std::string::clear();
          std::vector<bool,_std::allocator<bool>_>::clear(&this->buf_index_);
        }
        bVar1 = true;
      }
    }
    else {
      OutputMsg(rmError,"init order error. on init item.");
      bVar1 = false;
    }
  }
  return bVar1;
}



// ===== ItemBag::NotifyAdd @ 006e35c4 =====

/* DWARF original prototype: void NotifyAdd(ItemBag * this, Item * item, bool showTip) */

void __thiscall ItemBag::NotifyAdd(ItemBag *this,Item *item,bool showTip)

{
  bool bVar1;
  int iVar2;
  int32_t iVar3;
  bool showTip_local;
  Item *item_local;
  ItemBag *this_local;
  
  bVar1 = Entity::IsInited(&this->actor_->super_Entity);
  if (bVar1) {
    if ((NotifyAdd(Item*,bool)::msg == '\0') &&
       (iVar2 = __cxa_guard_acquire(&NotifyAdd(Item*,bool)::msg), iVar2 != 0)) {
      Protocol::BagAddItemProto::BagAddItemProto(&NotifyAdd::lexical_block_0::msg);
      __cxa_guard_release(&NotifyAdd(Item*,bool)::msg);
    }
    iVar3 = GetType(this);
    NotifyAdd::lexical_block_0::msg.bag_type = (uint8_t)iVar3;
    Item::WriteData(item,&NotifyAdd::lexical_block_0::msg.item);
    NotifyAdd::lexical_block_0::msg.show_tip = showTip;
    Actor::SendData<Protocol::BagAddItemProto>(this->actor_,&NotifyAdd::lexical_block_0::msg);
  }
  return;
}



// ===== ItemBag::NotifyDelete @ 006e367a =====

/* DWARF original prototype: void NotifyDelete(ItemBag * this, int64_t uid, int32_t count) */

void __thiscall ItemBag::NotifyDelete(ItemBag *this,int64_t uid,int32_t count)

{
  int iVar1;
  int32_t iVar2;
  int32_t count_local;
  int64_t uid_local;
  ItemBag *this_local;
  
  if (NotifyDelete(long,int)::msg == '\0') {
    iVar1 = __cxa_guard_acquire(&NotifyDelete(long,int)::msg);
    if (iVar1 != 0) {
      Protocol::BagDeleteItemProto::BagDeleteItemProto(&NotifyDelete::lexical_block_0::msg);
      __cxa_guard_release(&NotifyDelete(long,int)::msg);
    }
  }
  iVar2 = GetType(this);
  NotifyDelete::lexical_block_0::msg.bag_type = (uint8_t)iVar2;
  NotifyDelete::lexical_block_0::msg.uid = uid;
  Actor::SendData<Protocol::BagDeleteItemProto>(this->actor_,&NotifyDelete::lexical_block_0::msg);
  return;
}



// ===== ItemBag::NotifyUpdate @ 006e36f4 =====

/* DWARF original prototype: void NotifyUpdate(ItemBag * this, int64_t uid, int32_t count, bool
   showTip) */

void __thiscall ItemBag::NotifyUpdate(ItemBag *this,int64_t uid,int32_t count,bool showTip)

{
  int iVar1;
  int32_t iVar2;
  bool showTip_local;
  int32_t count_local;
  int64_t uid_local;
  ItemBag *this_local;
  
  if ((NotifyUpdate(long,int,bool)::msg == '\0') &&
     (iVar1 = __cxa_guard_acquire(&NotifyUpdate(long,int,bool)::msg), iVar1 != 0)) {
    Protocol::BagUpdateItemProto::BagUpdateItemProto(&NotifyUpdate::lexical_block_0::msg);
    __cxa_guard_release(&NotifyUpdate(long,int,bool)::msg);
  }
  iVar2 = GetType(this);
  NotifyUpdate::lexical_block_0::msg.bag_type = (uint8_t)iVar2;
  NotifyUpdate::lexical_block_0::msg.uid = uid;
  NotifyUpdate::lexical_block_0::msg.count = count;
  NotifyUpdate::lexical_block_0::msg.show_tip = showTip;
  Actor::SendData<Protocol::BagUpdateItemProto>(this->actor_,&NotifyUpdate::lexical_block_0::msg);
  return;
}



// ===== ItemBag::NotifyUpdateData @ 006e3794 =====

/* DWARF original prototype: void NotifyUpdateData(ItemBag * this, int64_t llUid, bool showTip) */

void __thiscall ItemBag::NotifyUpdateData(ItemBag *this,int64_t llUid,bool showTip)

{
  int iVar1;
  int32_t iVar2;
  Item *this_00;
  bool showTip_local;
  int64_t llUid_local;
  ItemBag *this_local;
  Item *item;
  
  this_00 = GetItem(this,llUid);
  if (this_00 == (Item *)0x0) {
    OutputMsg(rmError,"NotifyUpdateData fail, can not find item,uid:%lld",llUid);
  }
  else {
    if ((NotifyUpdateData(long,bool)::msg == '\0') &&
       (iVar1 = __cxa_guard_acquire(&NotifyUpdateData(long,bool)::msg), iVar1 != 0)) {
      Protocol::BagUpdateItemDataProto::BagUpdateItemDataProto
                (&NotifyUpdateData::lexical_block_0::msg);
      __cxa_guard_release(&NotifyUpdateData(long,bool)::msg);
    }
    iVar2 = GetType(this);
    NotifyUpdateData::lexical_block_0::msg.bag_type = (uint8_t)iVar2;
    Item::WriteData(this_00,&NotifyUpdateData::lexical_block_0::msg.item);
    NotifyUpdateData::lexical_block_0::msg.show_tip = showTip;
    Actor::SendData<Protocol::BagUpdateItemDataProto>
              (this->actor_,&NotifyUpdateData::lexical_block_0::msg);
  }
  return;
}



// ===== ItemBag::OnAddItem @ 006e386c =====

/* DWARF original prototype: void OnAddItem(ItemBag * this, Item * item) */

void __thiscall ItemBag::OnAddItem(ItemBag *this,Item *item)

{
  int32_t iVar1;
  ScriptValueList *params_00;
  Item *item_local;
  ItemBag *this_local;
  ScriptValueList params;
  
  if (item != (Item *)0x0) {
    ScriptValueList::ScriptValueList(&params);
    ScriptValueList::operator<<_<Actor*>(&params,this->actor_);
    ScriptValueList::operator<<_<tagActorEventID>(&params,aeGetItem);
    iVar1 = Item::GetType(item);
    ScriptValueList::operator<<_<int>(&params,iVar1);
    iVar1 = Item::GetId(item);
    ScriptValueList::operator<<_<int>(&params,iVar1);
    params_00 = &params;
                    /* try { // try from 006e3936 to 006e393a has its CatchHandler @ 006e394c */
    Actor::OnEvent(this->actor_,params_00,(ScriptValueList *)0x0);
    ScriptValueList::~ScriptValueList(&params,(int)params_00);
  }
  return;
}



// ===== ItemBag::OnEnterGame @ 006e3974 =====

/* DWARF original prototype: void OnEnterGame(ItemBag * this) */

void __thiscall ItemBag::OnEnterGame(ItemBag *this)

{
  int iVar1;
  int32_t iVar2;
  size_t sVar3;
  DataPacket *this_00;
  reference ppIVar4;
  ItemBag *this_local;
  ActorPacket AP;
  DataPacket *sp;
  int32_t i;
  int16_t index;
  int32_t count;
  
  if (OnEnterGame()::msg == '\0') {
    iVar1 = __cxa_guard_acquire(&OnEnterGame()::msg);
    if (iVar1 != 0) {
      Protocol::BagInitProto::BagInitProto(&OnEnterGame::lexical_block_0::msg);
      __cxa_guard_release(&OnEnterGame()::msg);
    }
  }
  iVar2 = GetType(this);
  OnEnterGame::lexical_block_0::msg.bag_type = (uint8_t)iVar2;
  sVar3 = BaseContainer::Count(&this->super_BaseContainer);
  count = (int32_t)sVar3;
  index = 0;
  while ((index == 0 || (0 < count))) {
    this_00 = Actor::AllocPacket(this->actor_,&AP);
    OnEnterGame::lexical_block_0::msg.init_index = (uint8_t)index;
    OnEnterGame::lexical_block_0::msg.item_count = 300;
    if (count < 0x12d) {
      OnEnterGame::lexical_block_0::msg.item_count = count;
    }
    count = count - OnEnterGame::lexical_block_0::msg.item_count;
    DataPacket::writeBuf(this_00,&OnEnterGame::lexical_block_0::msg,8);
    for (i = 0; i < OnEnterGame::lexical_block_0::msg.item_count; i = i + 1) {
      ppIVar4 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                          (&(this->super_BaseContainer).item_list_,(long)(i + index * 300));
      Item::WriteData(*ppIVar4,this_00);
    }
    ActorPacket::flush(&AP);
    index = index + 1;
  }
  return;
}



// ===== ItemBag::Save @ 006e3ab0 =====

/* DWARF original prototype: void Save(ItemBag * this, ActorBasicData * data, bool force) */

void __thiscall ItemBag::Save(ItemBag *this,ActorBasicData *data,bool force)

{
  int32_t sid;
  uint uVar1;
  int iVar2;
  GameServer *this_00;
  reference ppIVar3;
  ItemData *pIVar4;
  ItemData *pIVar5;
  size_type sVar6;
  long lVar7;
  DataPacket *pDVar8;
  void *pvVar9;
  ulong uVar10;
  ulong uVar11;
  bool bVar12;
  byte bVar13;
  bool force_local;
  ActorBasicData *data_local;
  ItemBag *this_local;
  uint32_t i_1;
  uint32_t local_60 [4];
  int32_t local_50;
  uint32_t local_4c [3];
  DataPacket *pack_1;
  DataPacket *pack;
  ItemData *cpy;
  DBClient *db;
  uint32_t i;
  
  bVar13 = 0;
  if (this->actor_ != (Actor *)0x0) {
    sid = Actor::GetServerId(this->actor_);
    this_00 = ServerInstance();
    db = GameServer::GetDbClient(this_00,sid);
    if ((db != (DBClient *)0x0) && ((force || ((this->super_BaseContainer).need_save_ == true)))) {
      std::vector<Item*,_std::allocator<Item*>_>::size(&(this->super_BaseContainer).item_list_);
      std::string::resize((ulong)&this->db_buf_);
      for (i = 0; uVar10 = (ulong)i,
          sVar6 = std::vector<Item*,_std::allocator<Item*>_>::size
                            (&(this->super_BaseContainer).item_list_), uVar10 < sVar6; i = i + 1) {
        cpy = (ItemData *)std::string::operator[]((ulong)&this->db_buf_);
        pIVar5 = cpy + i;
        ppIVar3 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                            (&(this->super_BaseContainer).item_list_,(ulong)i);
        pIVar4 = Item::GetItemData(*ppIVar3);
        uVar1 = 0x90;
        bVar12 = ((ulong)pIVar5 & 1) != 0;
        if (bVar12) {
          *(char *)&pIVar5->uid = (char)pIVar4->uid;
          pIVar5 = (ItemData *)((long)&pIVar5->uid + 1);
          pIVar4 = (ItemData *)((long)&pIVar4->uid + 1);
          uVar1 = 0x8f;
        }
        if (((ulong)pIVar5 & 2) != 0) {
          *(short *)&pIVar5->uid = (short)pIVar4->uid;
          pIVar5 = (ItemData *)((long)&pIVar5->uid + 2);
          pIVar4 = (ItemData *)((long)&pIVar4->uid + 2);
          uVar1 = uVar1 - 2;
        }
        if (((ulong)pIVar5 & 4) != 0) {
          *(int *)&pIVar5->uid = (int)pIVar4->uid;
          pIVar5 = (ItemData *)((long)&pIVar5->uid + 4);
          pIVar4 = (ItemData *)((long)&pIVar4->uid + 4);
          uVar1 = uVar1 - 4;
        }
        for (uVar10 = (ulong)(uVar1 >> 3); uVar10 != 0; uVar10 = uVar10 - 1) {
          pIVar5->uid = pIVar4->uid;
          pIVar4 = (ItemData *)((long)pIVar4 + (ulong)bVar13 * -0x10 + 8);
          pIVar5 = (ItemData *)((long)pIVar5 + (ulong)bVar13 * -0x10 + 8);
        }
        lVar7 = 0;
        if ((uVar1 & 4) != 0) {
          *(int *)&pIVar5->uid = (int)pIVar4->uid;
          lVar7 = 4;
        }
        if ((uVar1 & 2) != 0) {
          *(undefined2 *)((long)((pIVar5->attr_data).equip_attr.attr + -2) + lVar7) =
               *(undefined2 *)((long)((pIVar4->attr_data).equip_attr.attr + -2) + lVar7);
          lVar7 = lVar7 + 2;
        }
        if (bVar12) {
          *(undefined1 *)((long)((pIVar5->attr_data).equip_attr.attr + -2) + lVar7) =
               *(undefined1 *)((long)((pIVar4->attr_data).equip_attr.attr + -2) + lVar7);
        }
      }
      lVar7 = std::string::size();
      if (lVar7 == 0) {
        pack = DBClient::allocProtoPacket(db,0x501);
        local_60[0] = 0;
        local_60[1] = 0;
        local_60[2] = 0;
        local_60[3] = (*(this->actor_->super_Entity)._vptr_Entity[0x19])(this->actor_);
        pDVar8 = DataPacket::operator<<_<int>(pack,(int *)(local_60 + 3));
        pDVar8 = DataPacket::operator<<_<int>(pDVar8,(int *)&(this->super_BaseContainer).field_0x24)
        ;
        pDVar8 = DataPacket::operator<<_<unsigned_int>(pDVar8,local_60 + 2);
        pDVar8 = DataPacket::operator<<_<unsigned_int>(pDVar8,local_60 + 1);
        DataPacket::operator<<_<unsigned_int>(pDVar8,local_60);
        WorkSocket::FlushProtoPacket
                  ((WorkSocket *)db,(DataPacket_conflict *)pack,
                   "/mnt/hgfs/invCodeCpp/gameworld/systems/item/container/item_bag.cc",0xbf);
      }
      for (i_1 = 0; uVar11 = (ulong)i_1, uVar10 = std::string::size(), uVar11 < uVar10;
          i_1 = i_1 + 50000) {
        pack_1 = DBClient::allocProtoPacket(db,0x501);
        local_50 = (*(this->actor_->super_Entity)._vptr_Entity[0x19])(this->actor_);
        pDVar8 = DataPacket::operator<<_<int>(pack_1,&local_50);
        pDVar8 = DataPacket::operator<<_<int>(pDVar8,(int *)&(this->super_BaseContainer).field_0x24)
        ;
        DataPacket::operator<<_<unsigned_int>(pDVar8,&i_1);
        uVar1 = i_1 + 50000;
        uVar10 = std::string::size();
        if (uVar1 < uVar10) {
          local_4c[1] = 50000;
          DataPacket::operator<<_<unsigned_int>(pack_1,local_4c + 1);
        }
        else {
          iVar2 = std::string::size();
          local_4c[0] = iVar2 - i_1;
          DataPacket::operator<<_<unsigned_int>(pack_1,local_4c);
        }
        local_4c[2] = std::string::size();
        DataPacket::operator<<_<unsigned_int>(pack_1,local_4c + 2);
        uVar1 = i_1 + 50000;
        uVar10 = std::string::size();
        if (uVar10 < uVar1) {
          lVar7 = std::string::size();
          uVar10 = (ulong)i_1;
          pvVar9 = (void *)std::string::operator[]((ulong)&this->db_buf_);
          DataPacket::writeBuf(pack_1,pvVar9,lVar7 - uVar10);
        }
        else {
          pvVar9 = (void *)std::string::operator[]((ulong)&this->db_buf_);
          DataPacket::writeBuf(pack_1,pvVar9,50000);
        }
        WorkSocket::FlushProtoPacket
                  ((WorkSocket *)db,(DataPacket_conflict *)pack_1,
                   "/mnt/hgfs/invCodeCpp/gameworld/systems/item/container/item_bag.cc",0xd8);
      }
      std::string::clear();
    }
  }
  return;
}



// ===== ItemBag::AppendItem @ 006e3f7c =====

/* WARNING: Struct "GameEngine": ignoring overlapping field "net_msg_route" */
/* DWARF original prototype: bool AppendItem(ItemBag * this, int32_t id, int32_t count, char * log,
   bool showTip) */

bool __thiscall ItemBag::AppendItem(ItemBag *this,int32_t id,int32_t count,char *log,bool showTip)

{
  Item *this_00;
  _func_int32_t_varargs *p_Var1;
  int32_t iVar2;
  int iVar3;
  uint uVar4;
  int32_t actorid;
  int32_t accountid;
  reference ppIVar5;
  int64_t iVar6;
  GameServer *this_01;
  char *pcVar7;
  GameEngine *this_02;
  ItemConfig *pIVar8;
  char *name;
  bool showTip_local;
  char *log_local;
  int32_t count_local;
  int32_t id_local;
  ItemBag *this_local;
  char id_str [64];
  char count_str [64];
  char level_str [64];
  ItemConfig *conf;
  int32_t level;
  Item *item;
  int32_t pos;
  int32_t newCount;
  
  iVar2 = FindItem(this,id);
  if (iVar2 < 0) {
    BaseContainer::AddItem(&this->super_BaseContainer,id,count,showTip);
  }
  else {
    ppIVar5 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,(long)iVar2);
    this_00 = *ppIVar5;
    iVar2 = Item::GetCount(this_00);
    newCount = iVar2 + count;
    if (0x40000000 < newCount) {
      newCount = 0x40000000;
    }
    Item::SetCount(this_00,newCount);
    (this->super_BaseContainer).need_save_ = true;
    p_Var1 = (this->super_BaseContainer)._vptr_BaseContainer[4];
    uVar4 = Item::GetCount(this_00);
    iVar6 = Item::GetUId(this_00);
    (*p_Var1)(this,iVar6,(ulong)uVar4,(ulong)showTip);
  }
  if (AppendItem(int,int,char_const*,bool)::log_sender == '\0') {
    iVar3 = __cxa_guard_acquire(&AppendItem(int,int,char_const*,bool)::log_sender);
    if (iVar3 != 0) {
      this_01 = ServerInstance();
      AppendItem::lexical_block_0::log_sender = GameServer::GetLogClient(this_01);
      __cxa_guard_release(&AppendItem(int,int,char_const*,bool)::log_sender);
    }
  }
  uVar4 = Actor::GetLevel(this->actor_);
  sprintf(id_str,"%d",(ulong)(uint)id);
  sprintf(count_str,"%d",(ulong)(uint)count);
  sprintf(level_str,"%d",(ulong)uVar4);
  pcVar7 = Actor::GetAccount(this->actor_);
  iVar2 = (*(this->actor_->super_Entity)._vptr_Entity[0x19])(this->actor_);
  LogSender::LogCounter
            (AppendItem::lexical_block_0::log_sender,iVar2,pcVar7,level_str,"give item",id_str,
             count_str,"",log,"itembag","","",1);
  if (AppendItem(int,int,char_const*,bool)::dkmLog == '\0') {
    iVar3 = __cxa_guard_acquire(&AppendItem(int,int,char_const*,bool)::dkmLog);
    if (iVar3 != 0) {
      this_02 = GetGameEngine();
      AppendItem::lexical_block_0::dkmLog = GameEngine::GetDKMlogMgr(this_02);
      __cxa_guard_release(&AppendItem(int,int,char_const*,bool)::dkmLog);
    }
  }
  pIVar8 = ItemConfigPool::GetItemConfig(id);
  if ((0 < count) && (pIVar8 != (ItemConfig *)0x0)) {
    iVar2 = pIVar8->type;
    pcVar7 = Actor::GetPf(this->actor_);
    name = Entity::GetEntityName(&this->actor_->super_Entity);
    actorid = (*(this->actor_->super_Entity)._vptr_Entity[0x19])(this->actor_);
    accountid = Actor::GetAccountId(this->actor_);
    DKMLogMgr::LogAddItem
              (AppendItem::lexical_block_0::dkmLog,accountid,actorid,name,uVar4,pcVar7,id,count,0,
               iVar2,log);
  }
  return true;
}



// ===== ItemBag::CostItem @ 006e4332 =====

/* DWARF original prototype: bool CostItem(ItemBag * this, int64_t uid, int32_t count) */

bool __thiscall ItemBag::CostItem(ItemBag *this,int64_t uid,int32_t count)

{
  bool bVar1;
  int32_t count_local;
  int64_t uid_local;
  ItemBag *this_local;
  
  bVar1 = BaseContainer::DeleteItem(&this->super_BaseContainer,uid,count);
  return bVar1;
}



// ===== ItemBag::FindItem @ 006e435e =====

/* DWARF original prototype: int32_t FindItem(ItemBag * this, int32_t id) */

int32_t __thiscall ItemBag::FindItem(ItemBag *this,int32_t id)

{
  int32_t iVar1;
  reference ppIVar2;
  size_type sVar3;
  int32_t id_local;
  ItemBag *this_local;
  size_t i;
  
  i = 0;
  while( true ) {
    sVar3 = std::vector<Item*,_std::allocator<Item*>_>::size
                      (&(this->super_BaseContainer).item_list_);
    if (sVar3 <= i) {
      return -1;
    }
    ppIVar2 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,i);
    iVar1 = Item::GetId(*ppIVar2);
    if (iVar1 == id) break;
    i = i + 1;
  }
  return (int32_t)i;
}



// ===== ItemBag::FindItemUid @ 006e43d0 =====

/* DWARF original prototype: int64_t FindItemUid(ItemBag * this, int32_t id) */

int64_t __thiscall ItemBag::FindItemUid(ItemBag *this,int32_t id)

{
  int32_t iVar1;
  reference ppIVar2;
  int64_t iVar3;
  size_type sVar4;
  int32_t id_local;
  ItemBag *this_local;
  size_t i;
  
  i = 0;
  while( true ) {
    sVar4 = std::vector<Item*,_std::allocator<Item*>_>::size
                      (&(this->super_BaseContainer).item_list_);
    if (sVar4 <= i) {
      return -1;
    }
    ppIVar2 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,i);
    iVar1 = Item::GetId(*ppIVar2);
    if (iVar1 == id) break;
    i = i + 1;
  }
  ppIVar2 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                      (&(this->super_BaseContainer).item_list_,i);
  iVar3 = Item::GetUId(*ppIVar2);
  return iVar3;
}



// ===== ItemBag::GetItem @ 006e4462 =====

/* DWARF original prototype: Item * GetItem(ItemBag * this, int64_t uid) */

Item * __thiscall ItemBag::GetItem(ItemBag *this,int64_t uid)

{
  reference ppIVar1;
  int64_t iVar2;
  size_type sVar3;
  int64_t uid_local;
  ItemBag *this_local;
  size_t i;
  
  i = 0;
  while( true ) {
    sVar3 = std::vector<Item*,_std::allocator<Item*>_>::size
                      (&(this->super_BaseContainer).item_list_);
    if (sVar3 <= i) {
      return (Item *)0x0;
    }
    ppIVar1 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,i);
    iVar2 = Item::GetUId(*ppIVar1);
    if (iVar2 == uid) break;
    i = i + 1;
  }
  ppIVar1 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                      (&(this->super_BaseContainer).item_list_,i);
  return *ppIVar1;
}



// ===== ItemBag::GMClear @ 006e44ec =====

/* DWARF original prototype: void GMClear(ItemBag * this) */

void __thiscall ItemBag::GMClear(ItemBag *this)

{
  int32_t count;
  size_type sVar1;
  reference ppIVar2;
  int64_t uid;
  ItemBag *this_local;
  int32_t i;
  
  sVar1 = std::vector<Item*,_std::allocator<Item*>_>::size(&(this->super_BaseContainer).item_list_);
  i = (int32_t)sVar1;
  while (i = i + -1, -1 < i) {
    ppIVar2 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,(long)i);
    count = Item::GetCount(*ppIVar2);
    ppIVar2 = std::vector<Item*,_std::allocator<Item*>_>::operator[]
                        (&(this->super_BaseContainer).item_list_,(long)i);
    uid = Item::GetUId(*ppIVar2);
    BaseContainer::DeleteItem(&this->super_BaseContainer,uid,count);
  }
  return;
}



// ===== ItemBag::GetType @ 006e45a8 =====

/* DWARF original prototype: int32_t GetType(ItemBag * this) */

int32_t __thiscall ItemBag::GetType(ItemBag *this)

{
  ItemBag *this_local;
  
  return *(int32_t *)&(this->super_BaseContainer).field_0x24;
}



