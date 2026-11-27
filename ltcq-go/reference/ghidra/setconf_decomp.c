// ===== Item::SetConf @ 0060e244 =====

/* DWARF original prototype: void SetConf(Item * this, ItemConfig * conf) */

void __thiscall Item::SetConf(Item *this,ItemConfig *conf)

{
  ItemConfig *conf_local;
  Item *this_local;
  
  this->config_ = conf;
  return;
}



// ===== Item::Init @ 006e7124 =====

/* DWARF original prototype: bool Init(Item * this) */

bool __thiscall Item::Init(Item *this)

{
  Item *this_local;
  
  return this->config_ != (ItemConfig *)0x0;
}



