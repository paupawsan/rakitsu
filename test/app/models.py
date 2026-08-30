from typing import Optional, List
from datetime import datetime, timezone
from sqlmodel import SQLModel, Field, Relationship

def _utcnow() -> datetime:
    # Naive on purpose: the mapped column is a plain (timezone-naive)
    # DateTime, and this fixture's engine (SQLite, see config.py) doesn't
    # preserve tzinfo across a round-trip even with an explicit
    # DateTime(timezone=True) column — a freshly-constructed instance and
    # one loaded back from the DB would otherwise carry different datetime
    # kinds, which raises on comparison. Stored value is still correct UTC
    # wall-clock either way.
    return datetime.now(timezone.utc).replace(tzinfo=None)

# ---------- User ----------
class UserBase(SQLModel):
    email: str = Field(index=True, unique=True)
    full_name: Optional[str] = None
    is_active: bool = Field(default=True)

class UserCreate(UserBase):
    password: str  # plaintext, will be hashed before persisting

class User(UserBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    hashed_password: str
    created_at: datetime = Field(default_factory=_utcnow)
    updated_at: datetime = Field(default_factory=_utcnow)

    # Relationships
    orders: List["Order"] = Relationship(back_populates="customer")
    reviews: List["Review"] = Relationship(back_populates="author")

class UserRead(UserBase):
    id: int
    created_at: datetime
    updated_at: datetime

# ---------- Category <-> Product (many-to-many) ----------
class ProductCategoryLink(SQLModel, table=True):
    product_id: Optional[int] = Field(default=None, foreign_key="product.id", primary_key=True)
    category_id: Optional[int] = Field(default=None, foreign_key="category.id", primary_key=True)

# ---------- Category ----------
class CategoryBase(SQLModel):
    name: str

class CategoryCreate(CategoryBase):
    pass

class Category(CategoryBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    description: Optional[str] = None
    products: List["Product"] = Relationship(back_populates="categories", link_model=ProductCategoryLink)

class CategoryRead(CategoryBase):
    id: int
    description: Optional[str] = None

# ---------- Product ----------
class ProductBase(SQLModel):
    name: str
    description: Optional[str] = None
    price: float
    stock_qty: int = Field(default=0)

class ProductCreate(ProductBase):
    pass

class Product(ProductBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    created_at: datetime = Field(default_factory=_utcnow)
    updated_at: datetime = Field(default_factory=_utcnow)
    categories: List["Category"] = Relationship(back_populates="products", link_model=ProductCategoryLink)
    reviews: List["Review"] = Relationship(back_populates="product")

class ProductRead(ProductBase):
    id: int
    created_at: datetime
    updated_at: datetime

# ---------- Order ----------
class OrderItemBase(SQLModel):
    product_id: int = Field(foreign_key="product.id")
    quantity: int

class OrderCreate(SQLModel):
    items: List[OrderItemBase]
    shipping_address: str

class Order(SQLModel, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    status: str = "pending"
    created_at: datetime = Field(default_factory=_utcnow)
    shipping_address: str
    customer_id: int = Field(foreign_key="user.id")
    customer: "User" = Relationship(back_populates="orders")
    items: List["OrderItem"] = Relationship(back_populates="order")

class OrderItem(OrderItemBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    order_id: int = Field(foreign_key="order.id")
    order: "Order" = Relationship(back_populates="items")

class OrderRead(SQLModel):
    id: int
    status: str
    created_at: datetime
    shipping_address: str
    items: List["OrderItemRead"]

class OrderItemRead(SQLModel):
    product_id: int
    quantity: int
    name: str
    price: float

# ---------- Review ----------
class ReviewBase(SQLModel):
    rating: int = Field(gt=0, lt=6)   # 1‑5 stars
    comment: Optional[str] = None

class ReviewCreate(ReviewBase):
    product_id: int
    author_id: int

class Review(ReviewBase, table=True):
    id: Optional[int] = Field(default=None, primary_key=True)
    created_at: datetime = Field(default_factory=_utcnow)
    product_id: int = Field(foreign_key="product.id")
    author_id: int = Field(foreign_key="user.id")
    product: "Product" = Relationship(back_populates="reviews")
    author: "User" = Relationship(back_populates="reviews")

class ReviewRead(ReviewBase):
    id: int
    created_at: datetime
